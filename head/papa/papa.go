package admin

import (
	"context"
	"time"

	"github.com/temoto/vender/head/state"
	"github.com/temoto/vender/talk"
	"google.golang.org/grpc"
)

var (
	// TODO connection
	client talk.PapaClient
)

func init() {
	state.RegisterStart(func(ctx context.Context) error {
		// TODO: get from config
		remoteAddress := "127.0.0.1:50051"

		// creds := credentials.NewClientTLSFromFile(certFile, "")
		// conn, _ := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(creds))

		conn, err := grpc.Dial(remoteAddress, grpc.WithInsecure())
		if err != nil {
			return err
		}
		client = talk.NewPapaClient(conn)

		go func() {
			pull, _ := client.Pull(context.Background())
			for {
				task, err := pull.Recv()
				if err != nil {
					// TODO handle error
					_ = err
				}

				// TODO execute later from on-disk queue
				switch task.GetName() {
				case "restart-head":
					go time.AfterFunc(5*time.Second, func() {
						state.Restart()
					})
				}

				// TODO write task to disk, then execute
				err = pull.Send(&talk.PapaTask{Id: task.Id})
				// TODO handle error
				_ = err
			}
		}()

		return nil
	})
}
