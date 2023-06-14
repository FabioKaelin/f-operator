package utils

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"time"
)

// struct with methods

type Log struct {
	random string
}

func Init() Log {
	// random hex string
	random := RandStringBytesMaskImprSrc(2)

	return Log{random: random}
}

func (l *Log) Info(msg ...any) {
	// array with all msg
	msgs := []any{}
	msgs = append(msgs, "INFO "+l.random+"")
	msgs = append(msgs, msg...)
	fmt.Println(msgs...)
}

func RandStringBytesMaskImprSrc(n int) string {
	var src = rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, (n+1)/2) // can be simplified to n/2 if n is always even

	if _, err := src.Read(b); err != nil {
		panic(err)
	}

	return hex.EncodeToString(b)[:n]
}
