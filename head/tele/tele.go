package tele

import (
	"context"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
	"github.com/temoto/spq"
	tele_api "github.com/temoto/vender/head/tele/api"
	tele_config "github.com/temoto/vender/head/tele/config"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

const (
	defaultStateInterval  = 5 * time.Minute
	defaultNetworkTimeout = 30 * time.Second
)

// Tele contract:
// - Init() fails only with invalid config, network issues ignored
// - Transaction/Error/Service/etc public API calls block at most for disk write
//   network may be slow or absent, messages will be delivered in background
// - Close() will block until all messages are delivered
// - Telemetry/Response messages delivered at least once
// - Status messages may be lost
type Tele struct { //nolint:maligned
	enabled       bool
	log           *log2.Log
	transport     Transporter
	q             *spq.Queue
	stateCh       chan tele_api.State
	stopCh        chan struct{}
	vmId          int32
	stateInterval time.Duration
	stat          tele_api.Stat
}

func (self *Tele) Init(ctx context.Context, log *log2.Log, teleConfig tele_config.Config) error {
	self.enabled = teleConfig.Enabled
	self.log = log.Clone(log2.LInfo)
	if teleConfig.LogDebug {
		self.log.SetLevel(log2.LDebug)
	}
	if !self.enabled {
		return nil
	}

	self.stopCh = make(chan struct{})
	self.stateCh = make(chan tele_api.State)
	self.vmId = int32(teleConfig.VmId)
	self.stateInterval = helpers.IntSecondDefault(teleConfig.StateIntervalSec, defaultStateInterval)
	self.stat.Locked_Reset()

	if teleConfig.PersistPath == "" {
		panic("code error must set teleConfig.PersistPath")
	}
	var err error
	self.q, err = spq.Open(teleConfig.PersistPath)
	if err != nil {
		return errors.Annotate(err, "tele queue")
	}

	willPayload := []byte{byte(tele_api.State_Disconnected)}
	// test code sets .transport
	if self.transport == nil { // production path
		self.transport = &transportMqtt{}
	}
	if err := self.transport.Init(ctx, log, teleConfig, self.onCommandMessage, willPayload); err != nil {
		return errors.Annotate(err, "tele transport")
	}

	go self.qworker()
	go self.stateWorker()
	self.stateCh <- tele_api.State_Boot
	return nil
}

func (self *Tele) Close() {
	close(self.stopCh)
	self.q.Close()
}

func (self *Tele) stateWorker() {
	const retryInterval = 17 * time.Second
	var b [1]byte
	var sent bool
	tmrRegular := time.NewTicker(self.stateInterval)
	tmrRetry := time.NewTicker(retryInterval)
	for {
		select {
		case next := <-self.stateCh:
			if next != tele_api.State(b[0]) {
				b[0] = byte(next)
				sent = self.transport.SendState(b[:])
			}

		case <-tmrRegular.C:
			sent = self.transport.SendState(b[:])

		case <-tmrRetry.C:
			if !sent {
				sent = self.transport.SendState(b[:])
			}

		case <-self.stopCh:
			return
		}
	}
}

// denote value type in persistent queue bytes form
const (
	qCommandResponse byte = 1
	qTelemetry       byte = 2
)

func (self *Tele) qworker() {
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
				err = self.q.Delete(box)
				if err != nil {
					self.log.Errorf("tele qhandle Delete b=%x err=%v", b, err)
				}
			} else {
				// FIXME delete+re-push atomically inside spq
				self.q.Delete(box)
				self.q.Push(b)
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

func (self *Tele) qhandle(b []byte) (bool, error) {
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

func (self *Tele) qpushCommandResponse(c *tele_api.Command, r *tele_api.Response) error {
	if c.ReplyTopic == "" {
		err := errors.Errorf("command with reply_topic=empty")
		self.Error(err)
		return err
	}
	r.INTERNALTopic = c.ReplyTopic
	return self.qpushTagProto(qCommandResponse, r)
}

func (self *Tele) qpushTelemetry(tm *tele_api.Telemetry) error {
	if tm.VmId == 0 {
		tm.VmId = self.vmId
	}
	self.stat.Lock()
	defer self.stat.Unlock()
	tm.Stat = &self.stat.Telemetry_Stat
	err := self.qpushTagProto(qTelemetry, tm)
	self.stat.Locked_Reset()
	return err
}

func (self *Tele) qpushTagProto(tag byte, pb proto.Message) error {
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

func (self *Tele) qsendResponse(r *tele_api.Response) bool {
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

func (self *Tele) qsendTelemetry(tm *tele_api.Telemetry) bool {
	payload, err := proto.Marshal(tm)
	if err != nil {
		self.log.Errorf("CRITICAL telemetry Marshal tm=%#v err=%v", tm, err)
		return true // retry will not help
	}
	// self.log.Debugf("SendTelemetry %x", payload)
	return self.transport.SendTelemetry(payload)
}
