package controllers

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"

	"github.com/Ouest-France/gofortiadc"
)

func NewFortiClient() (gofortiadc.Client, error) {

	address := os.Getenv("FORTIADC_ADDRESS")
	username := os.Getenv("FORTIADC_USERNAME")
	password := os.Getenv("FORTIADC_PASSWORD")
	if address == "" || username == "" || password == "" {
		return gofortiadc.Client{}, errors.New("Env vars FORTIADC_ADDRESS, FORTIADC_USERNAME and FORTIADC_PASSWORD must be set")
	}

	// Construct an http client with cookies
	cookieJar, _ := cookiejar.New(nil)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{},
	}
	httpClient := &http.Client{
		Jar:       cookieJar,
		Transport: tr,
	}

	// Construct new forti Client instance
	fortiClient := gofortiadc.Client{
		Client:   httpClient,
		Address:  address,
		Username: username,
		Password: password,
	}

	// Send auth request to get authorization token
	err := fortiClient.Login()
	if err != nil {
		return gofortiadc.Client{}, fmt.Errorf("failed to login to fortiadc: %w", err)
	}

	return fortiClient, nil
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}
