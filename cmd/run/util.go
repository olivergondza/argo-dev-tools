package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
)

// checkMarker checks if a regular file exists at filePath
// and contains at least one line matching regexPattern.
func checkMarker(filePath string, regexPattern *regexp.Regexp) error {
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("not Run from project directory. file does not exist: %s", filePath)
		}
		return fmt.Errorf("error accessing file info: %w", err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("not Run from project directory. path is not a regular file: %s", filePath)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("error opening file %s: %w", filePath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if regexPattern.MatchString(scanner.Text()) {
			return nil
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading file %s: %w", filePath, err)
	}

	return fmt.Errorf("not Run from project directory")
}

func checkDocker() error {
	mp := NewManagedProc([]string{"docker", "ps"}...)
	if err := mp.Run(); err != nil {
		return fmt.Errorf("docker not running: %s", err)
	}

	return nil
}

// getOutboundIP returns the preferred outbound IP for connecting to the internet.
func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}
