package nginxhealthz

import (
	"fmt"
)

func RunServer() error {
	fmt.Println("service start")
	if err := run(); err != nil {
		return err
	}
	return nil
}

func run() error {
	return nil
}
