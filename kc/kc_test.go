package main

import (
	"bytes"
	"crypto/rand"
	"net/http"
	"testing"
)

func BenchmarkKcpTransporter_Dial(b *testing.B) {

	sendData := make([]byte, 128)
	rand.Read(sendData)

	//初始化tr实例
	tr := &kcpTransporter{
		sessions: make(map[string]*muxSession),
	}
	server, err := tr.Dial("127.0.0.1:7777")
	if err != nil {
		b.Error(err)
	}
	defer server.Close()

	for i := 0; i < b.N; i++ {
		req, err := http.NewRequest(
			http.MethodGet,
			"https://127.0.0.1:9999",
			bytes.NewReader(sendData),
		)

		if err != nil {
			return
		}

		if err := req.Write(server); err != nil {
			b.Error(err)
		}
	}

}
