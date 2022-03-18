package main

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/smux"
	// "github.com/xtaci/kcp-go/v5"
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

type kcpListener struct {
	ln       *kcp.Listener
	connChan chan net.Conn
	errChan  chan error
}

func main() {

	ln, err := kcp.ListenWithOptions(":7777", nil, 10, 3)
	if err != nil {
		log.Println("error listen ", err)
		return
	}
	defer ln.Close()

	l := &kcpListener{
		ln:       ln,
		connChan: make(chan net.Conn, 1024),
		errChan:  make(chan error, 1),
	}

	go l.listenLoop()

	log.Println("开启监听端口 " + "7777")
	var tempDelay time.Duration
	for {
		client, e := l.Accept()
		if e != nil {
			if ne, ok := e.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				log.Println("server: Accept error: %v; retrying in %v", e, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return
		}
		tempDelay = 0
		go handle(client)
	}

}

func (l *kcpListener) listenLoop() {
	for {
		conn, err := l.ln.AcceptKCP() //此处阻塞
		if err != nil {
			log.Println("error Accept ", err)
			l.errChan <- err
			close(l.errChan)
			return
		}

		go l.mux(conn)

	}
}

func (l *kcpListener) mux(conn net.Conn) {
	mux, err := smux.Server(conn, nil)
	if err != nil {
		log.Println("[kcp]", err)
		return
	}
	defer mux.Close()

	for {
		stream, err := mux.AcceptStream()
		if err != nil {
			log.Println("error AcceptStream ", err)
			return
		}
		select {
		case l.connChan <- stream:
		default:
			stream.Close()
			log.Println("full")
		}
	}

}

func (l *kcpListener) Accept() (conn net.Conn, err error) {
	var ok bool
	select {
	case conn = <-l.connChan:
	case err, ok = <-l.errChan:
		if !ok {
			err = errors.New("accpet on closed listener")
		}
	}
	return
}

func handle(client net.Conn) {
	i++
	fmt.Println("====================i=", i, "==============================")
	defer client.Close()

	req, err := http.ReadRequest(bufio.NewReader(client))
	if err != nil {
		log.Println("(1):req read err:", err)
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
		server, err = net.Dial("tcp", host)
		if err != nil {
			log.Println("(2):Dial err:", err)
			time.Sleep(10 * time.Millisecond)
		} else {
			break
		}

	}

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

	defer server.Close()

	if req.Method == http.MethodConnect {
		b := []byte("HTTP/1.1 200 Connection established\r\n" +
			"Proxy-Agent: test" + "\r\n\r\n")
		client.Write(b)
		if err != nil {
			log.Println("(3):", err)
		}
	} else {
		req.Header.Del("Proxy-Connection")
		fmt.Println(req.Method)
		if err = req.WriteProxy(server); err != nil { //将client发来的请求，指向服务器
			log.Printf("(4):[http] %s -> %s : %s\n", client.RemoteAddr(), client.LocalAddr(), err)
			return
		}
	}

	fmt.Printf("(5):[http] %s <-> %s\n", client.RemoteAddr(), host)

	err = transport(client, server)
	if err != nil {
		log.Println("(6):Copy Error:", err)
	}

	fmt.Printf("(7)[http] %s >-< %s\n", client.RemoteAddr(), host)

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
