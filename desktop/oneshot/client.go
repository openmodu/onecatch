package main

import "os"

func apiBaseURL() string {
	if value := os.Getenv("ONESHOT_API_BASE_URL"); value != "" {
		return value
	}
	return "http://127.0.0.1:8080"
}
