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

var mockData = map[string]MockResponse{
	// Non streaming:
	"793764f12a5e331ae08cecab749a022c23867d03c9db18cf00fc4dd1dc89f132": {FilePath: "mocks/routing/azure_non_streaming.json", IsGzip: false},
	"dfb4094b64f15e250490d4f6f8a3163c840b4cff09f0c282d41765f0a1d8a7f5": {FilePath: "mocks/routing/openai_non_streaming.txt.gz", IsGzip: true},
	"c9c34d39cb0af7ef19530a58aae8557d951fb1eef1fcaf2b65583cb823ca47a2": {FilePath: "mocks/routing/gemini_non_streaming.json", IsGzip: false},
	"6be80eb5071d90b7aafefc1e2f11d045acec300c1c71e6bbfce415bb3ede0abd": {FilePath: "mocks/routing/vertex_ai_non_streaming.json", IsGzip: false},
	// Streaming:
	"daa5badeb5cfabcb85b36bb0d6d8daa2a63536329f3c48e654137a6b3dc8c3d6": {FilePath: "mocks/streaming/azure_streaming.txt", IsGzip: false},
	"0e065e8eedf476d066f55668fadb4626ee47fb6452baaadf636366866c2582bf": {FilePath: "mocks/streaming/openai_streaming.txt", IsGzip: false},
	"3c8b0bd3db97733f4a4f1a4214f392b6193577a69da5e908f3d16a74b369024e": {FilePath: "mocks/streaming/gemini_streaming.txt", IsGzip: false},
	"15044ae8bdb808e1a5cd1aff384464ad5ed9d25f164261b4ea3c287c2153d9e8": {FilePath: "mocks/streaming/vertex_ai_streaming.txt", IsGzip: false},
	// Prompt Guard:
	"dd472364fe55fcf75df24cd2b7cb1a32f9f8e4d36477c5ba7960de9f112a2d32": {FilePath: "mocks/promptguard/openai-mask.json", IsGzip: false},
	"512b50a42206d1a4cc2d7609e6e34b7a23123234bc6b3d682d04c4e6e1d5d401": {FilePath: "mocks/promptguard/vertex-ai.json", IsGzip: false},
	// Prompt Guard Streaming:
	"4e1c1a0b4f697df2fe9ad3ac4898dcffcff881e85f271d63c229d208cb60c59c": {FilePath: "mocks/promptguard-streaming/openai-mask.txt", IsGzip: false},
	"38e35f6adfbc50177014a04cf6484c7cf6b91cc7ccf1328ca246519892cbfd53": {FilePath: "mocks/promptguard-streaming/openai-no-guard.txt", IsGzip: false},
	"6b473280b6aa5b35b8de94f6a5212811f8fb624785695b3234cf9a88d3075b38": {FilePath: "mocks/promptguard-streaming/vertex-ai-mask.txt", IsGzip: false},
	"0d86bb4c7d7d3638e251c1fc6d09ef515df3aec19d3a10895dd75c4e30ff505e": {FilePath: "mocks/promptguard-streaming/vertex-ai-no-guard.txt", IsGzip: false},
}

func getJSONHash(data map[string]interface{}, provider string, stream bool) string {
	data["provider"] = provider
	data["stream"] = stream

	jsonBytes, _ := json.Marshal(data)
	hash := sha256.Sum256(jsonBytes)
	return fmt.Sprintf("%x", hash[:])
}

func generateSSEStream(c *gin.Context, filePath, provider string) {
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("failed to open file: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to open file: %v", err)})
		return
	}
	defer file.Close()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if provider == "vertex_ai" || provider == "gemini" {
			line := scanner.Text()
			sseMessage := fmt.Sprintf("data: %s\n\r\n\r\n", line)

			// Write directly to response
			c.Writer.WriteString(sseMessage)
			c.Writer.Flush()
		} else {
			line := scanner.Text()
			sseMessage := fmt.Sprintf("data: %s\n\n", line)

			// Write directly to response
			c.Writer.WriteString(sseMessage)
			c.Writer.Flush()
		}
	}
	if provider == "openai" || provider == "azure" {
		lastSseMessage := fmt.Sprintf("data: [DONE]\n\n")
		c.Writer.WriteString(lastSseMessage)
		c.Writer.Flush()
	}
}

func handleModelResponse(c *gin.Context, requestData map[string]interface{}, provider string, stream bool) {
	hash := getJSONHash(requestData, provider, stream)
	fmt.Printf("data: %v, hash: %s\n", requestData, hash)

	if response, exists := mockData[hash]; exists {
		if stream {
			generateSSEStream(c, response.FilePath, provider)
			return
		}

		if response.IsGzip {
			c.Header("Content-Encoding", "gzip")
		}
		c.File(response.FilePath)
	} else {
		fmt.Errorf("Mock response not found for data: %v, hash: %s\n", requestData, hash)
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

	// Custom endpoints
	r.POST("/api/v1/chat/completions", func(c *gin.Context) {
		var requestData map[string]interface{}
		c.BindJSON(&requestData)
		stream := false
		if requestData["stream"] != nil {
			stream, _ = requestData["stream"].(bool)
			fmt.Printf("has stream: %v\n", stream)
		}
		// check that api token is provided
		apiToken := c.Request.Header.Get("custom-header")
		if apiToken != "custom-prefix" {
			fmt.Println("no api token provided in header")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "API token is required"})
			return
		}
		handleModelResponse(c, requestData, "openai", stream)
	})

	// OpenAI endpoints
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		var requestData map[string]interface{}
		c.BindJSON(&requestData)
		stream := false
		if requestData["stream"] != nil {
			stream, _ = requestData["stream"].(bool)
			fmt.Printf("has stream: %v\n", stream)
		}
		// check that api token is provided
		apiToken := c.Request.Header.Get("Authorization")
		if apiToken == "" {
			fmt.Println("no api token provided in header")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "API token is required"})
			return
		}
		handleModelResponse(c, requestData, "openai", stream)
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
			fmt.Printf("has stream: %v\n", stream)
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

		apiToken := c.Query("key")
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
		fmt.Printf("no route %s\n", c.Request.URL.Path)
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "Page not found",
			"path":    c.Request.URL.Path,
			"method":  c.Request.Method,
			"headers": c.Request.Header,
		})
	})

	srv := &http.Server{
		Addr:      ":443",
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
