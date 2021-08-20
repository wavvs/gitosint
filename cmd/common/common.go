package common

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"
)

var encoder *json.Encoder

func (r *GitRecon) Write() error {
	return encoder.Encode(r)
}

func (r *GitRecon) SetError(err error) {
	if err != nil {
		r.Time = time.Now()
		r.Error = append(r.Error, &Error{Message: err.Error()})
	}
}

func SetOutput(path string) error {
	if path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		encoder = json.NewEncoder(f)
	} else {
		encoder = json.NewEncoder(os.Stdout)
	}
	return nil
}

func ReadFile(path string) ([]string, error) {
	var lines []string
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if scanner.Err() != nil {
		return nil, scanner.Err()
	}
	return lines, nil
}

func PrintJSON(item interface{}) {
	b, err := json.Marshal(item)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(b))
}
