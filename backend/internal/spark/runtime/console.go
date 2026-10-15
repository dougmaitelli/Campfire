package runtime

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// Input writes to game stdin without closing or replacing the game process.
func (Docker) Input(ctx context.Context, container, text string) error {
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}

	if !strings.HasPrefix(host, "unix://") || os.Getenv("DOCKER_CONTEXT") != "" {
		return errors.New("console input requires the local Docker Unix socket")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", strings.TrimPrefix(host, "unix://"))
	if err != nil {
		return errors.New("could not connect to Docker console")
	}

	defer func() { _ = connection.Close() }()

	stop := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stop()

	deadline, _ := ctx.Deadline()
	if err := connection.SetDeadline(deadline); err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://docker/containers/"+container+"/attach?stream=1&stdin=1&stdout=0&stderr=0", nil)
	if err != nil {
		return err
	}

	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "tcp")

	if err := request.Write(connection); err != nil {
		return err
	}

	response, err := http.ReadResponse(bufio.NewReader(connection), request)
	if err != nil {
		return err
	}

	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusSwitchingProtocols {
		return errors.New("Docker rejected console attachment")
	}

	_, err = io.WriteString(connection, text+"\n")

	return err
}
