package input

import (
	"io"
	"os"

	"github.com/temoto/inputevent-go"
	"github.com/temoto/vender/internal/types"
)

const DevInputEventTag = "dev-input-event"

type DevInputEventSource struct {
	f io.ReadCloser
}

// compile-time interface compliance test
var _ Source = new(DevInputEventSource)

func (self *DevInputEventSource) String() string { return DevInputEventTag }

func NewDevInputEventSource(device string) (*DevInputEventSource, error) {
	f, err := os.Open(device)
	if err != nil {
		return nil, err
	}
	return &DevInputEventSource{f: f}, nil
}

func (self *DevInputEventSource) Read() (types.InputEvent, error) {
	for {
		ie, err := inputevent.ReadOne(self.f)
		if err != nil {
			// g.Log.Errorf("%s err=%v", DevInputEventTag, err)
			return types.InputEvent{}, err
		}
		if ie.Type == inputevent.EV_KEY {
			// g.Log.Debugf("%s key=%v", DevInputEventTag, ie.Code)
			ev := types.InputEvent{
				Source: DevInputEventTag,
				Key:    types.InputKey(ie.Code),
				Up:     ie.Value == int32(inputevent.KeyStateUp),
			}
			return ev, nil
		}
	}
}
