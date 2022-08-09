package k8sadmission

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/solo-io/gloo/pkg/utils"
	"go.opencensus.io/tag"
)

type certificateProvider struct {
	ctx       context.Context
	logger    *log.Logger
	cert      unsafe.Pointer //of type *tls.Certificate
	certPath  string
	keyPath   string
	certMtime time.Time
	keyMtime  time.Time
}

func NewCertificateProvider(certPath, keyPath string, logger *log.Logger, ctx context.Context, interval time.Duration) (*certificateProvider, error) {
	mReloadSuccess := utils.MakeSumCounter("validation.gateway.solo.io/certificate_reload_success", "Number of successful certificate reloads")
	mReloadFailed := utils.MakeSumCounter("validation.gateway.solo.io/certificate_reload_failed", "Number of failed certificate reloads")
	tagKey, err := tag.NewKey("error")
	if err != nil {
		return nil, err
	}
	certFileInfo, err := os.Stat(certPath)
	if err != nil {
		return nil, err
	}
	keyFileInfo, err := os.Stat(keyPath)
	if err != nil {
		return nil, err
	}
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	utils.MeasureOne(ctx, mReloadSuccess)
	result := &certificateProvider{
		ctx:       ctx,
		logger:    logger,
		cert:      unsafe.Pointer(&cert),
		certPath:  certPath,
		keyPath:   keyPath,
		certMtime: certFileInfo.ModTime(),
		keyMtime:  keyFileInfo.ModTime(),
	}
	go func() {
		result.logger.Println("start validating admission webhook certificate change watcher goroutine")
		for ctx.Err() == nil {
			// Kubelet caches Secrets and therefore has some delay until it realizes
			// that a Secret has changed and applies the update to the mounted secret files.
			// So, we can safely sleep some time here to safe CPU/IO resources and do not
			// have to spin in a tight loop, watching for changes.
			time.Sleep(interval)
			if ctx.Err() != nil {
				// Avoid error messages if Context has been cancelled while we were sleeping (best effort).
				break
			}
			certFileInfo, err := os.Stat(certPath)
			if err != nil {
				result.logger.Printf("Error while checking if validating admission webhook certificate file changed %s", err)
				utils.MeasureOne(ctx, mReloadFailed, tag.Insert(tagKey, fmt.Sprintf("%s", err)))
				continue
			}
			keyFileInfo, err := os.Stat(keyPath)
			if err != nil {
				result.logger.Printf("Error while checking if validating admission webhook private key file changed %s", err)
				utils.MeasureOne(ctx, mReloadFailed, tag.Insert(tagKey, fmt.Sprintf("%s", err)))
				continue
			}
			km := keyFileInfo.ModTime()
			cm := certFileInfo.ModTime()
			if result.keyMtime != km || result.certMtime != cm {
				err := result.reload()
				if err == nil {
					result.logger.Println("Reloaded validating admission webhook certificate")
					result.keyMtime = km
					result.certMtime = cm
					utils.MeasureOne(ctx, mReloadSuccess)
				} else {
					result.logger.Printf("Error while reloading validating admission webhook certificate %s, will keep using the old certificate", err)
					utils.MeasureOne(ctx, mReloadFailed, tag.Insert(tagKey, fmt.Sprintf("%s", err)))
				}
			}
		}
		result.logger.Println("terminate validating admission webhook certificate change watcher goroutine")
	}()
	return result, nil
}

func (p *certificateProvider) reload() error {
	newCert, err := tls.LoadX509KeyPair(p.certPath, p.keyPath)
	if err != nil {
		return err
	}
	atomic.StorePointer(&p.cert, unsafe.Pointer(&newCert))
	return nil
}

func (p *certificateProvider) GetCertificateFunc() func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		return (*tls.Certificate)(atomic.LoadPointer(&p.cert)), nil
	}
}
