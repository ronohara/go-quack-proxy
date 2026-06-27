package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

func main() {
	// Create log file
	logFile := "test.log"
	dir := filepath.Dir(logFile)
	os.MkdirAll(dir, 0755)
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// Create multi-writer
	writer := io.MultiWriter(os.Stdout, file)
	handler := slog.NewTextHandler(writer, nil)
	logger := slog.New(handler)

	logger.Info("This should go to both stdout and file")
	logger.Info("Second message")

	println("Done. Check test.log")
}
