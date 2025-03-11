package main

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

type MockResponse struct {
	FilePath string
	IsGzip   bool
}

var testData = map[string]MockResponse{
	"793764f12a5e331ae08cecab749a022c23867d03c9db18cf00fc4dd1dc89f132": {FilePath: "data/routing/azure_non_streaming.json", IsGzip: false},
	"fdcaa093f659f4035e1502c2d7b4ed8160365330513b20ec1deed795327037b3": {FilePath: "data/routing/openai_non_streaming.txt.gz", IsGzip: true},
	"c9c34d39cb0af7ef19530a58aae8557d951fb1eef1fcaf2b65583cb823ca47a2": {FilePath: "data/routing/gemini_non_streaming.json", IsGzip: false},
	"6be80eb5071d90b7aafefc1e2f11d045acec300c1c71e6bbfce415bb3ede0abd": {FilePath: "data/routing/vertex_ai_non_streaming.json", IsGzip: false},
	"daa5badeb5cfabcb85b36bb0d6d8daa2a63536329f3c48e654137a6b3dc8c3d6": {FilePath: "data/streaming/azure_streaming.txt", IsGzip: false},
	"705bf37e4ef6d83df189e431aeb6515ac101cce05bbd0056d8aa33da140c724b": {FilePath: "data/streaming/openai_streaming.txt", IsGzip: false},
	"3cfe127aeb62bea0bd5f716e2cb41cde7ee716f10253fdaa5ce635e112396e86": {FilePath: "data/streaming/gemini_streaming.txt", IsGzip: false},
	"932f03e0388bfffb32732bf96e2aa76f31967c8e8f073ed835092c2e1146cfa6": {FilePath: "data/streaming/vertex_ai_streaming.txt", IsGzip: false},
}

func getJSONHash(data map[string]interface{}, provider string, stream bool) string {
	data["provider"] = provider
	data["stream"] = stream

	jsonBytes, _ := json.Marshal(data)
	hash := sha256.Sum256(jsonBytes)
	return fmt.Sprintf("%x", hash[:])
}

func generateSSEStream(c *gin.Context, filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		return
	}
	defer file.Close()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		c.SSEvent("", scanner.Text())
		time.Sleep(100 * time.Millisecond) // Simulate delay between chunks
	}
}

func handleModelResponse(c *gin.Context, requestData map[string]interface{}, provider string, stream bool) {
	hash := getJSONHash(requestData, provider, stream)
	fmt.Printf("data: %v, hash: %s\n", requestData, hash)

	if response, exists := testData[hash]; exists {
		if stream {
			generateSSEStream(c, response.FilePath)
			return
		}

		if response.IsGzip {
			c.Header("Content-Encoding", "gzip")
		}
		c.File(response.FilePath)
	} else {
		c.JSON(http.StatusNotFound, gin.H{"message": "Mock response not found"})
	}
}

func main() {
	r := gin.Default()

	// Health check endpoint
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "healthy",
			"service": "mock-provider",
		})
	})

	// OpenAI endpoints
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		var requestData map[string]interface{}
		c.BindJSON(&requestData)
		stream := false
		if requestData["stream"] != nil {
			stream, _ = requestData["stream"].(bool)
			print("has stream: ", stream)
		}
		// check that api token is provided
		apiToken := c.Request.Header.Get("Authorization")
		if apiToken == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "API token is required"})
			return
		}
		handleModelResponse(c, requestData, "azure", stream)
	})

	// Azure OpenAI endpoints
	r.POST("/openai/deployments/gpt-4o-mini/chat/completions", func(c *gin.Context) {
		apiVersion := c.Query("api-version")
		if apiVersion == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "API version should be set"})
			return
		}

		var requestData map[string]interface{}
		if err := c.ShouldBindJSON(&requestData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		stream := false
		if requestData["stream"] != nil {
			stream, _ = requestData["stream"].(bool)
			print("has stream: ", stream)
		}
		// check that api token is provided
		apiToken := c.Request.Header.Get("api-key")
		if apiToken == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "API token is required"})
			return
		}
		handleModelResponse(c, requestData, "azure", stream)
	})

	// Gemini endpoints
	r.POST("/v1beta/models/gemini-1.5-flash-001:action", func(c *gin.Context) {
		var requestData map[string]interface{}
		if err := c.BindJSON(&requestData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		apiToken := c.Param("key")
		if apiToken == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "API token is required"})
			return
		}

		action := c.Param("action")
		if action == ":generateContent" {
			handleModelResponse(c, requestData, "gemini", false)
		} else if action == ":streamGenerateContent" {
			handleModelResponse(c, requestData, "gemini", true)
		} else {
			c.JSON(http.StatusNotFound, gin.H{"error": "Invalid action"})
		}
	})

	// Vertex AI endpoints
	r.POST("/v1/projects/kgateway-project/locations/us-central1/publishers/google/models/gemini-1.5-flash-001:action", func(c *gin.Context) {
		var requestData map[string]interface{}
		if err := c.BindJSON(&requestData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		apiToken := c.Request.Header.Get("Authorization")
		if apiToken == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "API token is required"})
			return
		}

		action := c.Param("action")
		if action == ":generateContent" {
			handleModelResponse(c, requestData, "vertex_ai", false)
		} else if action == ":streamGenerateContent" {
			handleModelResponse(c, requestData, "vertex_ai", true)
		} else {
			c.JSON(http.StatusNotFound, gin.H{"error": "Invalid action"})
		}
	})

	// Add NoRoute handler for debugging
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "Page not found",
			"path":    c.Request.URL.Path,
			"method":  c.Request.Method,
			"headers": c.Request.Header,
		})
	})

	srv := &http.Server{
		Addr:      ":5001",
		Handler:   r,
		TLSConfig: generateTLSConfig(),
	}

	if err := srv.ListenAndServeTLS("", ""); err != nil {
		panic(err)
	}
}

func generateTLSConfig() *tls.Config {
	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Mock Server"},
			CommonName:   "localhost",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		panic(err)
	}

	// Create TLS certificate
	cert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privateKey,
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
}
