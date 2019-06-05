package tele

import (
	"fmt"

	proto "github.com/golang/protobuf/proto"
)

const logMsgDisabled = "tele disabled"

func (self *Tele) CommandChan() <-chan Command {
	if !self.Enabled {
		self.Log.Errorf(logMsgDisabled)
		return nil
	}

	return self.cmdCh
}

func (self *Tele) CommandReplyErr(c *Command, e error) {
	if !self.Enabled {
		self.Log.Errorf(logMsgDisabled)
		return
	}

	if c.ReplyTopic == "" {
		self.Log.Errorf("CommandReplyErr with empty reply_topic")
		return
	}
	r := Response{
		CommandId: c.Id,
		Error:     e.Error(),
	}
	b, err := proto.Marshal(&r)
	if err != nil {
		// TODO panic?
		self.Log.Errorf("CommandReplyErr proto.Marshal err=%v")
		return
	}

	topic := fmt.Sprintf("%s/%s", self.topicPrefix, c.ReplyTopic)
	self.m.Publish(topic, 1, false, b)
}

func (self *Tele) StatModify(fun func(s *Stat)) {
	if !self.Enabled {
		self.Log.Errorf(logMsgDisabled)
		return
	}

	self.stat.Lock()
	fun(&self.stat)
	self.stat.Unlock()
}

func (self *Tele) Transaction() {
	if !self.Enabled {
		self.Log.Errorf(logMsgDisabled)
		return
	}

}

func (self *Tele) Error(err error) {
	if !self.Enabled {
		self.Log.Errorf(logMsgDisabled)
		return
	}

	self.stateCh <- State_Problem
	// FIXME send err
	self.Log.Errorf("tele.Error err=%v", err)
}

func (self *Tele) Service(msg string) {
	if !self.Enabled {
		self.Log.Errorf(logMsgDisabled)
		return
	}

	self.stateCh <- State_Service
	// FIXME send msg
	self.Log.Infof("tele.Service msg=%s", msg)
}
