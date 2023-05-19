package v1helpers

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/test/helpers"
)

func StartSslProxy(ctx context.Context, port uint32) uint32 {
	return StartSslProxyWithHelloCB(ctx, port, nil)
}

func StartSslProxyWithHelloCB(ctx context.Context, port uint32, cb func(chi *tls.ClientHelloInfo)) uint32 {
	cert := []byte(helpers.Certificate())
	key := []byte(helpers.PrivateKey())
	cer, err := tls.X509KeyPair(cert, key)
	Expect(err).NotTo(HaveOccurred())

	config := &tls.Config{
		GetCertificate: func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
			if cb != nil {
				cb(chi)
			}
			return &cer, nil
		},
	}
	listener, err := tls.Listen("tcp", ":0", config)
	Expect(err).NotTo(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		<-ctx.Done()
		listener.Close()
	}()

	go func() {
		defer GinkgoRecover()
		for {
			conn, err := listener.Accept()
			if ctx.Err() != nil {
				return
			}
			Expect(err).NotTo(HaveOccurred())
			go func() {
				defer GinkgoRecover()
				proxyConnection(ctx, conn, port)
			}()
		}
	}()

	addr := listener.Addr().String()
	_, portstr, err := net.SplitHostPort(addr)
	Expect(err).NotTo(HaveOccurred())

	lport, err := strconv.Atoi(portstr)
	Expect(err).NotTo(HaveOccurred())

	fmt.Fprintf(GinkgoWriter, "starting ssl proxy to port %v to port %v\n", port, lport)
	return uint32(lport)
}

func proxyConnection(ctx context.Context, conn net.Conn, port uint32) {
	defer conn.Close()
	fmt.Fprintf(GinkgoWriter, "proxing connection to to port %v\n", port)
	defer fmt.Fprintf(GinkgoWriter, "proxing connection to to port %v done\n", port)

	c, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", port))
	Expect(err).NotTo(HaveOccurred())
	defer c.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	copythings := func(dst io.Writer, src io.Reader) {
		defer cancel()
		fmt.Fprintf(GinkgoWriter, "proxing copying started\n")
		w, err := io.Copy(dst, src)
		fmt.Fprintf(GinkgoWriter, "proxing copying return w: %v err %v\n", w, err)
	}

	go copythings(conn, c)
	go copythings(c, conn)
	<-ctx.Done()
}
