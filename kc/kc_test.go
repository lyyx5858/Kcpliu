package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"io/ioutil"
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
		//fmt.Println(i)

		if err != nil {
			b.Error(err)
			return
		}

		if err := req.Write(server); err != nil {
			b.Error(err)
			return
		}

		resp, err := http.ReadResponse(bufio.NewReader(server), req)
		if err != nil {
			b.Error(err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b.Error(resp.Status)
			return
		}

		recv, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			b.Error(err)
			return
		}

		if !bytes.Equal(sendData, recv) {
			b.Error("data not equal")
			return
		}
	}

}
