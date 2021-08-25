package certprovider

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

type CertificateProvider struct {
	ctx       context.Context
	logger    *log.Logger
	cert      unsafe.Pointer // of type *tls.Certificate
	certPath  string
	keyPath   string
	certMtime time.Time
	keyMtime  time.Time
}

func NewCertificateProvider(validatorName string, certPath, keyPath string, logger *log.Logger, ctx context.Context, interval time.Duration) (*CertificateProvider, error) {
	mReloadSuccess := utils.MakeSumCounter(fmt.Sprintf("validation.%s.solo.io/certificate_reload_success", validatorName), "Number of successful certificate reloads")
	mReloadFailed := utils.MakeSumCounter(fmt.Sprintf("validation.%s.solo.io/certificate_reload_failed", validatorName), "Number of failed certificate reloads")
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
	result := &CertificateProvider{
		ctx:       ctx,
		logger:    logger,
		cert:      unsafe.Pointer(&cert),
		certPath:  certPath,
		keyPath:   keyPath,
		certMtime: certFileInfo.ModTime(),
		keyMtime:  keyFileInfo.ModTime(),
	}
	go func() {
		result.logger.Printf("start %s validating admission webhook certificate change watcher goroutine", validatorName)
		for ctx.Err() == nil {
			// Kublet caches Secrets and therefore has some delay until it realizes
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
				result.logger.Printf("Error while checking if %s validating admission webhook certificate file changed %s", validatorName, err)
				utils.MeasureOne(ctx, mReloadFailed, tag.Insert(tagKey, fmt.Sprintf("%s", err)))
				continue
			}
			keyFileInfo, err := os.Stat(keyPath)
			if err != nil {
				result.logger.Printf("Error while checking if %s validating admission webhook private key file changed %s", validatorName, err)
				utils.MeasureOne(ctx, mReloadFailed, tag.Insert(tagKey, fmt.Sprintf("%s", err)))
				continue
			}
			km := keyFileInfo.ModTime()
			cm := certFileInfo.ModTime()
			if result.keyMtime != km || result.certMtime != cm {
				err := result.reload()
				if err == nil {
					result.logger.Printf("Reloaded %s validating admission webhook certificate", validatorName)
					result.keyMtime = km
					result.certMtime = cm
					utils.MeasureOne(ctx, mReloadSuccess)
				} else {
					result.logger.Printf("Error while reloading %s validating admission webhook certificate %s, will keep using the old certificate", validatorName, err)
					utils.MeasureOne(ctx, mReloadFailed, tag.Insert(tagKey, fmt.Sprintf("%s", err)))
				}
			}
		}
		result.logger.Printf("terminate %s validating admission webhook certificate change watcher goroutine", validatorName)
	}()
	return result, nil
}

func (p *CertificateProvider) reload() error {
	newCert, err := tls.LoadX509KeyPair(p.certPath, p.keyPath)
	if err != nil {
		return err
	}
	atomic.StorePointer(&p.cert, unsafe.Pointer(&newCert))
	return nil
}

func (p *CertificateProvider) GetCertificateFunc() func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		return (*tls.Certificate)(atomic.LoadPointer(&p.cert)), nil
	}
}
