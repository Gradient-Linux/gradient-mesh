package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/Gradient-Linux/gradient-mesh/internal/mesh"
)

func main() {
	code := run(context.Background(), os.Args[1:])
	os.Exit(code)
}

func run(ctx context.Context, args []string) int {
	cmd := "run"
	if len(args) > 0 {
		cmd = args[0]
	}

	switch cmd {
	case "run":
		fs := flag.NewFlagSet("run", flag.ContinueOnError)
		workspace := fs.String("workspace", mesh.DefaultWorkspaceRoot(), "workspace root")
		socketPath := fs.String("socket", mesh.DefaultSocketPath, "unix socket path")
		servicePort := fs.Int("port", mesh.DefaultServicePort, "mDNS advertisement port")
		visibility := fs.String("visibility", string(mesh.VisibilityPublic), "default visibility override")
		fs.SetOutput(os.Stderr)
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		cfg := mesh.DefaultMeshConfig()
		cfg.WorkspaceRoot = *workspace
		cfg.SocketPath = *socketPath
		cfg.ServicePort = *servicePort
		cfg.Visibility = mesh.NodeVisibility(*visibility)
		if err := mesh.Run(ctx, cfg); err != nil && err != context.Canceled {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	case "version", "--version", "-v":
		fmt.Println("gradient-mesh dev")
		return 0
	case "help", "--help", "-h":
		fmt.Println("usage: gradient-mesh [run|version]")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		return 1
	}
}
