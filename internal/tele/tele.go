package tele

import (
	"context"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
	"github.com/temoto/spq"
	"github.com/temoto/vender/log2"
	tele_api "github.com/temoto/vender/tele"
	tele_config "github.com/temoto/vender/tele/config"
)

const (
	defaultStateInterval  = 5 * time.Minute
	DefaultNetworkTimeout = 30 * time.Second
)

// Tele contract:
// - Init() fails only with invalid config, network issues ignored
// - Transaction/Error/Service/etc public API calls block at most for disk write
//   network may be slow or absent, messages will be delivered in background
// - Close() will block until all messages are delivered
// - Telemetry/Response messages delivered at least once
// - Status messages may be lost
type tele struct { //nolint:maligned
	config    tele_config.Config
	log       *log2.Log
	transport Transporter
	q         *spq.Queue
	// stateCh      chan tele_api.State
	stopCh       chan struct{}
	vmId         int32
	stat         tele_api.Stat
	currentState tele_api.State
}

func New() tele_api.Teler {
	return &tele{}
}
func NewWithTransporter(trans Transporter) tele_api.Teler {
	return &tele{transport: trans}
}

func (self *tele) Init(ctx context.Context, log *log2.Log, teleConfig tele_config.Config) error {
	self.config = teleConfig
	self.log = log
	if self.config.LogDebug {
		self.log.SetLevel(log2.LDebug)
	}

	// self.stopCh = make(chan struct{})
	// self.stateCh = make(chan tele_api.State)
	self.vmId = int32(self.config.VmId)
	self.stat.Locked_Reset()

	// willPayload := []byte{byte(tele_api.State_Disconnected)}
	// test code sets .transport
	if self.transport == nil { // production path
		self.transport = &transportMqtt{}
	}
	if err := self.transport.Init(ctx, log, teleConfig, self.onCommandMessage); err != nil {
		return errors.Annotate(err, "tele transport")
	}
	if !self.config.Enabled {
		return nil
	}

	if self.config.PersistPath == "" {
		panic("code error must set self.config.PersistPath")
	}
	var err error
	self.q, err = spq.Open(self.config.PersistPath)
	if err != nil {
		return errors.Annotate(err, "tele queue")
	}

	go self.qworker()
	self.State(tele_api.State_Boot)

	return nil
}

func (self *tele) Close() {
	// close(self.stopCh)
	if self.q != nil {
		self.q.Close()
	}
	self.transport.CloseTele()
}

// denote value type in persistent queue bytes form
const (
	qCommandResponse byte = 1
	qTelemetry       byte = 2
)

func (self *tele) qworker() {
	for {
		box, err := self.q.Peek()
		switch err {
		case nil:
			// success path
			b := box.Bytes()
			// self.log.Debugf("q.peek %x", b)
			var del bool
			del, err = self.qhandle(b)
			if err != nil {
				self.log.Errorf("tele qhandle b=%x err=%v", b, err)
			}
			if del {
				if err = self.q.Delete(box); err != nil {
					self.log.Errorf("tele qhandle Delete b=%x err=%v", b, err)
				}
			} else {
				if err = self.q.DeletePush(box); err != nil {
					self.log.Errorf("tele qhandle DeletePush b=%x err=%v", b, err)
				}
			}

		case spq.ErrClosed:
			select {
			case <-self.stopCh: // success path
			default:
				self.log.Errorf("CRITICAL tele spq closed unexpectedly")
				// TODO try to send telemetry?
			}
			return

		default:
			self.log.Errorf("CRITICAL tele spq err=%v", err)
			// here will go yet unhandled shit like disk full
		}
	}
}

func (self *tele) qhandle(b []byte) (bool, error) {

	if len(b) == 0 {
		self.log.Errorf("tele spq peek=empty")
		// what else can we do?
		return true, nil
	}

	switch b[0] {
	case qCommandResponse:
		var r tele_api.Response
		if err := proto.Unmarshal(b[1:], &r); err != nil {
			return true, err
		}
		return self.qsendResponse(&r), nil

	case qTelemetry:
		var tm tele_api.Telemetry
		if err := proto.Unmarshal(b[1:], &tm); err != nil {
			return true, err
		}
		return self.qsendTelemetry(&tm), nil

	default:
		err := errors.Errorf("unknown kind=%d", b[0])
		return true, err
	}
}

func (self *tele) qpushCommandResponse(c *tele_api.Command, r *tele_api.Response) error {
	// c.ReplyTopic = "cr"
	// r.INTERNALTopic = c.ReplyTopic
	r.INTERNALTopic = "cr"

	return self.qpushTagProto(qCommandResponse, r)
}

func (self *tele) qpushTelemetry(tm *tele_api.Telemetry) error {
	if tm.VmId == 0 {
		tm.VmId = self.vmId
	}
	if tm.Time == 0 {
		tm.Time = time.Now().UnixNano()
	}
	self.stat.Lock()
	defer self.stat.Unlock()
	tm.Stat = &self.stat.Telemetry_Stat
	err := self.qpushTagProto(qTelemetry, tm)
	self.stat.Locked_Reset()
	return err
}

func (self *tele) qpushTagProto(tag byte, pb proto.Message) error {
	buf := proto.NewBuffer(make([]byte, 0, 1024))
	if err := buf.EncodeVarint(uint64(tag)); err != nil {
		return err
	}
	if err := buf.Marshal(pb); err != nil {
		return err
	}
	// self.log.Debugf("qpushTagProto %x", buf.Bytes())
	return self.q.Push(buf.Bytes())
}

func (self *tele) qsendResponse(r *tele_api.Response) bool {
	// do not serialize INTERNAL_topic field
	wireResponse := *r
	wireResponse.INTERNALTopic = ""
	payload, err := proto.Marshal(&wireResponse)
	if err != nil {
		self.log.Errorf("CRITICAL response Marshal r=%#v err=%v", r, err)
		return true // retry will not help
	}
	return self.transport.SendCommandResponse(r.INTERNALTopic, payload)
}

func (self *tele) qsendTelemetry(tm *tele_api.Telemetry) bool {
	payload, err := proto.Marshal(tm)
	if err != nil {
		self.log.Errorf("CRITICAL telemetry Marshal tm=%#v err=%v", tm, err)
		return true // retry will not help
	}
	// self.log.Debugf("SendTelemetry %x", payload)
	return self.transport.SendTelemetry(payload)
}
