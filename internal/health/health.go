// Package health provides HTTP health check for Quack endpoints.
package health

import (
	"fmt"
	"net/http"
	"time"
)

// Check performs a single HTTP GET health check against a Quack endpoint.
// Returns true if the endpoint responds with a 2xx status within timeout.
func Check(host string, port int, path string, timeout time.Duration) bool {
	url := fmt.Sprintf("http://%s:%d%s", host, port, path)
	client := &http.Client{Timeout: timeout}

	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// CheckWithLog performs a health check and returns the result along with status code and error.
func CheckWithLog(host string, port int, path string, timeout time.Duration) (bool, int, error) {
	url := fmt.Sprintf("http://%s:%d%s", host, port, path)
	client := &http.Client{Timeout: timeout}

	resp, err := client.Get(url)
	if err != nil {
		return false, 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300, resp.StatusCode, nil
}