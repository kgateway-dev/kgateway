package k8sadmisssion

import (
	"context"
	"crypto/tls"
	"log"
	"os"
	"sync/atomic"
	"time"
	"unsafe"
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

func NewCertificateProvider(certPath, keyPath string, logger *log.Logger, ctx context.Context) (*certificateProvider, error) {
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
			// Kublet caches Secrets and therefore has some delay until it realizes
			// that a Secret has changed and applies the update to the mounted secret files.
			// So, we can safely sleep some time here to safe CPU/IO resources and do not
			// have to spin in a tight loop, watching for changes.
			time.Sleep(10 * time.Second)
			certFileInfo, err := os.Stat(certPath)
			if err != nil {
				result.logger.Printf("Error while checking if validating admission webhook certificate file changed %s", err)
				continue
			}
			keyFileInfo, err := os.Stat(keyPath)
			if err != nil {
				result.logger.Printf("Error while checking if validating admission webhook private key file changed %s", err)
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
				} else {
					result.logger.Printf("Error while reloading validating admission webhook certificate %s, will keep using the old certificate", err)
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
