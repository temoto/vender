package tele

import (
	"fmt"

	proto "github.com/golang/protobuf/proto"
)

func (self *Tele) CommandChan() <-chan Command { return self.cmdCh }
func (self *Tele) CommandReplyErr(c *Command, e error) {
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
	self.stat.Lock()
	fun(&self.stat)
	self.stat.Unlock()
}

func (self *Tele) Transaction() {
}

func (self *Tele) Error(err error) {
	self.stateCh <- State_Problem
}
