package papa

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/temoto/vender/head/state"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

//go:generate protoc -I=../../protobuf --go_out=plugins=grpc:./ ../../protobuf/papa.proto

func netLoop(ctx context.Context) {
	// TODO alive
	for {
		client, err := dial(ctx)
		if err == nil {
			err = network(ctx, client)
		}
		if err != nil {
			log.Print(err)
			time.Sleep(1 * time.Second)
			if neterr, ok := err.(net.Error); ok {
				if neterr.Temporary() {
					continue
				} else {
					log.Fatal("network error is permanent")
					return
				}
			}
		}
	}
}

func network(ctx context.Context, client PapaClient) error {
	pull, err := client.Pull(ctx)
	if err != nil {
		return err
	}
	for {
		task, err := pull.Recv()
		if err != nil {
			log.Print(err)
			// TODO handle error
			time.Sleep(1 * time.Second)
			continue
		}

		// TODO execute later from on-disk queue
		switch task.GetName() {
		case "restart-head":
			go time.AfterFunc(5*time.Second, func() {
				state.Restart()
			})
		}

		// TODO write task to disk, then execute
		err = pull.Send(&PapaTask{Id: task.Id})
		// TODO handle error
		_ = err
	}
}

func dial(ctx context.Context) (PapaClient, error) {
	config := state.GetConfig(ctx)

	optSecurity := grpc.WithInsecure()
	if config.Papa.CertFile != "" {
		creds, err := credentials.NewClientTLSFromFile(config.Papa.CertFile, "")
		if err != nil {
			log.Print(err)
			return nil, err
		}
		optSecurity = grpc.WithTransportCredentials(creds)
	}

	conn, err := grpc.Dial(config.Papa.Address, optSecurity)
	if err != nil {
		log.Print(err)
		return nil, err
	}

	client := NewPapaClient(conn)
	return client, nil
}

func onStart(ctx context.Context) error {
	// TODO alive
	// FIXME temp disabled
	// go netLoop(ctx)
	return nil
}

func init() {
	state.RegisterStart(onStart)
}
