package iodin

import (
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
)

const modName string = "iodin-client"

//go:generate protoc -I=../../protobuf --go_out=./ ../../protobuf/iodin.proto

type Client struct {
	proc     *os.Process
	rf       *os.File
	wf       *os.File
	refcount int32
}

func NewClient(path string) (*Client, error) {
	// one pipe to send data to iodin and one to receive
	fSendRead, fSendWrite, err := os.Pipe()
	if err != nil {
		return nil, errors.Trace(err)
	}
	fRecvRead, fRecvWrite, err := os.Pipe()
	if err != nil {
		return nil, errors.Trace(err)
	}

	attr := &os.ProcAttr{
		Env:   nil,
		Files: []*os.File{fSendRead, fRecvWrite, os.Stderr},
	}
	p, err := os.StartProcess(path, nil, attr)
	if err != nil {
		return nil, errors.Trace(err)
	}

	c := &Client{
		proc: p,
		rf:   fRecvRead,
		wf:   fSendWrite,
	}
	return c, nil
}

func (self *Client) Close() error {
	r := Request{
		Command: Request_STOP,
	}
	err := self.Do(&r, new(Response))
	if err != nil {
		log.Printf("%s Close() Do(STOP) error=%v", modName, err)
	}
	self.rf.Close()
	self.wf.Close()
	err = self.proc.Signal(syscall.SIGTERM)
	if err != nil {
		log.Printf("%s Close() Signal(SIGTERM) error=%v", modName, err)
	}
	done := make(chan error)
	go func() {
		_, err := self.proc.Wait()
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		log.Printf("%s graceful SIGTERM timeout, begin hard kill", modName)
	}
	self.proc.Kill()
	_, err = self.proc.Wait()
	return err
}

func (self *Client) IncRef(debug string) {
	log.Printf("%s incref by %s", modName, debug)
	atomic.AddInt32(&self.refcount, 1)
}
func (self *Client) DecRef(debug string) error {
	log.Printf("%s decref by %s", modName, debug)
	new := atomic.AddInt32(&self.refcount, -1)
	switch {
	case new > 0:
		return nil
	case new == 0:
		return self.Close()
	}
	panic(fmt.Sprintf("code error %s decref<0 debug=%s", modName, debug))
}

func (self *Client) Do(request *Request, response *Response) error {
	// sock.SetDeadline(time.Now().Add(5*time.Second))
	// defer sock.SetDeadline(time.Time{})
	buf := make([]byte, 256)
	pb := proto.NewBuffer(buf[:0])
	{
		pb.EncodeFixed32(uint64(proto.Size(request)))
		err := pb.Marshal(request)
		if err != nil {
			return errors.Annotatef(err, "iodin.Do.Marshal req=%s", request.String())
		}
		_, err = self.wf.Write(pb.Bytes())
		if err != nil {
			return errors.Annotatef(err, "iodin.Do.Write req=%s", request.String())
		}
	}

	n, err := self.rf.Read(buf[:4])
	if err != nil || n < 4 {
		return errors.Annotatef(err, "iodin.Do.Read len buf=%x n=%d/4 req=%s", buf[:n], n, request.String())
	}
	pb.SetBuf(buf[:n])
	lu64, err := pb.DecodeFixed32()
	responseLen := int(lu64)
	if err != nil {
		return errors.Annotatef(err, "iodin.Do.Read len decode buf=%x req=%s", buf[:n], request.String())
	}
	if responseLen > len(buf) {
		return errors.Errorf("iodin.Do.Read buf overflow %d>%d req=%s", responseLen, len(buf), request.String())
	}
	n, err = self.rf.Read(buf[:responseLen])
	if err != nil {
		return errors.Annotatef(err, "iodin.Do.Read response buf=%x req=%s", buf[:n], request.String())
	}
	if n < responseLen {
		return errors.NotImplementedf("iodin.Do.Read response did not fit in one read() syscall len=%d req=%s", responseLen, request.String())
	}
	pb.SetBuf(buf[:n])
	err = pb.Unmarshal(response)
	if err != nil {
		return errors.Annotatef(err, "iodin.Do.Unmarshal buf=%x req=%s", pb.Bytes(), request.String())
	}
	return nil
}
