package httpclient

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go-novel-reader/config"
)

// Client HTTP客户端封装
type Client struct {
	client      *http.Client
	config      *config.AppConfig
	userAgents  []string
}

// NewClient 创建HTTP客户端
func NewClient(cfg *config.AppConfig) *Client {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     30 * time.Second,
	}

	// 配置代理
	if cfg.ProxyEnabled == 1 && cfg.ProxyHost != "" {
		proxyURL, err := url.Parse(fmt.Sprintf("http://%s:%d", cfg.ProxyHost, cfg.ProxyPort))
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return &Client{
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		config:     cfg,
		userAgents: defaultUserAgents,
	}
}

// Get 发送GET请求
func (c *Client) Get(url string, timeout int) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	c.setDefaultHeaders(req)

	if timeout > 0 {
		c.client.Timeout = time.Duration(timeout) * time.Second
	}

	return c.client.Do(req)
}

// GetWithCookies 发送带Cookie的GET请求
func (c *Client) GetWithCookies(url string, cookies string, timeout int) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	c.setDefaultHeaders(req)
	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}

	if timeout > 0 {
		c.client.Timeout = time.Duration(timeout) * time.Second
	}

	return c.client.Do(req)
}

// Post 发送POST请求
func (c *Client) Post(url string, data url.Values, timeout int) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	c.setDefaultHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if timeout > 0 {
		c.client.Timeout = time.Duration(timeout) * time.Second
	}

	return c.client.Do(req)
}

// PostWithCookies 发送带Cookie的POST请求
func (c *Client) PostWithCookies(url string, data url.Values, cookies string, timeout int) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	c.setDefaultHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}

	if timeout > 0 {
		c.client.Timeout = time.Duration(timeout) * time.Second
	}

	return c.client.Do(req)
}

// setDefaultHeaders 设置默认请求头
func (c *Client) setDefaultHeaders(req *http.Request) {
	req.Header.Set("User-Agent", c.randomUserAgent())
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Connection", "keep-alive")
}

// randomUserAgent 随机获取User-Agent
func (c *Client) randomUserAgent() string {
	return c.userAgents[rand.Intn(len(c.userAgents))]
}

// RandomInterval 生成随机间隔时间
func (c *Client) RandomInterval() time.Duration {
	interval := c.config.MinInterval + rand.Intn(c.config.MaxInterval-c.config.MinInterval)
	return time.Duration(interval) * time.Millisecond
}

// RandomRetryInterval 生成随机重试间隔时间
func (c *Client) RandomRetryInterval() time.Duration {
	interval := c.config.RetryMinInterval + rand.Intn(c.config.RetryMaxInterval-c.config.RetryMinInterval)
	return time.Duration(interval) * time.Millisecond
}

// ReadBody 读取响应体并关闭
func ReadBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// 默认User-Agent列表
var defaultUserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Safari/605.1.15",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
}
