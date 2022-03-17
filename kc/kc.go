package main

import (
	"bufio"
	"fmt"
	"github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/smux"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

var (
	lPool = sync.Pool{
		New: func() interface{} {
			p := make([]byte, 16*1024)
			return &p
		},
	}
)

var i int

func main() {
	var port string = "8080"

	laddr, err := net.ResolveTCPAddr("tcp", ":8080")
	li, err := net.ListenTCP("tcp", laddr)
	if err != nil {
		log.Println("error listen ", err)
		return
	}
	defer li.Close()

	log.Println("开启监听端口 " + port)

	var tempDelay time.Duration
	for {
		client, err := li.AcceptTCP() //此处阻塞
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				log.Printf("server: Accept error: %v; retrying in %v\n", err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return
		}
		tempDelay = 0

		go handle(client)
	}
}

func handle(client net.Conn) {
	i++
	fmt.Println("====================i=", i, "==============================")
	defer client.Close()

	req, err := http.ReadRequest(bufio.NewReader(client))
	if err != nil {
		log.Println("1:req read err:", err)
		return
	}
	defer req.Body.Close()

	host := req.Host
	fmt.Printf("HOST is %s\n", host)
	if _, port, _ := net.SplitHostPort(host); port == "" {
		host = net.JoinHostPort(host, "80")
	}

	var server net.Conn

	retry := 3

	for i := 0; i < retry; i++ {

		//kcpconn, err := kcp.NewConn(":7777", nil, 10, 3, c1)
		kcpconn, err := kcp.DialWithOptions("127.0.0.1:7777", nil, 10, 3)

		if err != nil {
			log.Println("error kcpconn ", err)
			return
		}
		session, err := smux.Client(kcpconn, nil)
		if err != nil {
			log.Println("error sumx.Clent ", err)
			return
		}

		stream, err := session.OpenStream()
		if err != nil {
			log.Println("error OpenStream ", err)
			return
		}
		server = net.Conn(stream)

		if err != nil {
			log.Println("2:Dial err:", err)
			time.Sleep(10 * time.Millisecond)
		} else {
			break
		}

	}

	defer server.Close()

	//如果尝试了retry次后，还是不成功，要向客户方写resp，说明不成功原因,然后返回。
	resp := &http.Response{
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{},
	}
	resp.Header.Add("Proxy-Agent", "test")

	if err != nil {
		resp.StatusCode = http.StatusServiceUnavailable
		resp.Write(client)
		return

	}

	err = req.Write(server) //将client发来的请求，指向服务器

	if err != nil {
		log.Printf("4:[http] %s -> %s : %s\n", client.RemoteAddr(), client.LocalAddr(), err)
		return
	}

	fmt.Printf("5:[http] %s <-> %s\n", client.RemoteAddr(), host)

	err = transport(client, server)
	if err != nil {
		log.Println("6:Copy Error:", err)
	}

	fmt.Printf("[http] %s >-< %s\n", client.RemoteAddr(), host)

}

func transport(rw1, rw2 io.ReadWriter) error {
	errc := make(chan error, 1)

	go func() {
		errc <- copyBuffer(rw1, rw2)
	}()

	go func() {
		errc <- copyBuffer(rw2, rw1)
	}()

	err := <-errc

	if err != nil && err == io.EOF {
		err = nil
	}
	return err
}

func copyBuffer(dst io.Writer, src io.Reader) error {
	buf := lPool.Get().(*[]byte)
	defer lPool.Put(buf)

	_, err := io.CopyBuffer(dst, src, *buf)
	return err
}
