package pkg

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// EnsureTLS 读取 TLS 证书，如果没有则自动生成自签证书
func EnsureTLS(certDir string) (string, string, error) {
	certPath := filepath.Join(certDir, "cert.pem")
	keyPath := filepath.Join(certDir, "key.pem")

	// 检查证书是否已存在
	if _, err := os.Stat(certPath); err == nil {
		if _, err := os.Stat(keyPath); err == nil {
			// 证书和密钥都存在，验证是否有效
			cert, err := tls.LoadX509KeyPair(certPath, keyPath)
			if err == nil {
				// 检查证书是否过期
				parsed, err := x509.ParseCertificate(cert.Certificate[0])
				if err == nil && time.Now().Before(parsed.NotAfter) {
					log.Println("使用现有 TLS 证书")
					return certPath, keyPath, nil
				}
			}
		}
	}

	// 生成自签证书
	log.Println("未找到有效 TLS 证书，自动生成自签证书")
	if err := generateSelfSignedCert(certPath, keyPath); err != nil {
		return "", "", fmt.Errorf("生成自签证书失败: %w", err)
	}

	log.Println("自签证书已生成（建议在生产环境中使用正式证书）")
	return certPath, keyPath, nil
}

// generateSelfSignedCert 生成自签证书
func generateSelfSignedCert(certPath, keyPath string) error {
	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
		return err
	}

	// 生成 ECDSA 私钥
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("生成私钥失败: %w", err)
	}

	// 生成序列号
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("生成序列号失败: %w", err)
	}

	// 获取本机 IP
	var ipAddrs []net.IP
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				ipAddrs = append(ipAddrs, ipNet.IP)
			}
		}
	}

	hostname, _ := os.Hostname()

	// 证书模板
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Server Probe"},
			CommonName:   hostname,
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		DNSNames:    []string{hostname, "localhost"},
		IPAddresses: append(ipAddrs, net.ParseIP("127.0.0.1")),
	}

	// 签名证书
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("签名证书失败: %w", err)
	}

	// 写入证书文件
	certOut, err := os.Create(certPath)
	if err != nil {
		return fmt.Errorf("创建证书文件失败: %w", err)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return fmt.Errorf("写入证书失败: %w", err)
	}

	// 写入私钥文件
	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("创建私钥文件失败: %w", err)
	}
	defer keyOut.Close()

	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("序列化私钥失败: %w", err)
	}

	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		return fmt.Errorf("写入私钥失败: %w", err)
	}

	return nil
}
