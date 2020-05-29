package cp

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/kubectl/pkg/cmd/exec"
	"k8s.io/kubectl/pkg/scheme"

	"github.com/kelda-inc/blimp/cli/authstore"
	"github.com/kelda-inc/blimp/cli/cp/kubectlcp"
	"github.com/kelda-inc/blimp/cli/manager"
	"github.com/kelda-inc/blimp/pkg/errors"
	"github.com/kelda-inc/blimp/pkg/kube"
)

func New() *cobra.Command {
	return &cobra.Command{
		Use:   "cp SRC DST",
		Short: "Copy files to and from services ",
		Long: `To copy FROM a container:
	blimp cp SERVICE:SRC_PATH DEST_PATH

To copy TO a container:
	blimp cp SRC_PATH SERVICE:DEST_PATH`,
		Run: func(_ *cobra.Command, args []string) {
			if len(args) != 2 {
				fmt.Fprintf(os.Stderr, "Dest path and src path need to be defined")
				os.Exit(1)
			}

			if err := run(args[0], args[1]); err != nil {
				errors.HandleFatalError(err)
			}
		},
	}
}

func run(src, dst string) error {
	auth, err := authstore.New()
	if err != nil {
		return errors.WithContext("parse auth config", err)
	}

	if auth.AuthToken == "" {
		fmt.Fprintln(os.Stderr, "Not logged in. Please run `blimp login`.")
		return nil
	}

	srcSpec, err := kubectlcp.ExtractFileSpec(src)
	if err != nil {
		return errors.WithContext("parse src", err)
	}
	destSpec, err := kubectlcp.ExtractFileSpec(dst)
	if err != nil {
		return errors.WithContext("parse dest", err)
	}

	if len(srcSpec.PodName) != 0 && len(destSpec.PodName) != 0 {
		// If both args look like remote paths, we assume src is local and dest
		// is remote. This matches what kubectl cp does.
		if _, err := os.Stat(src); err != nil {
			return errors.New("src doesn't exist in local filesystem")
		}
		srcSpec = kubectlcp.FileSpec{File: src}
	}

	kubeClient, restConfig, err := auth.KubeClient()
	if err != nil {
		return errors.WithContext("get kube client", err)
	}

	// Required by `kubectlcp` to access the Kubernetes API.
	restConfig.GroupVersion = &schema.GroupVersion{Group: "", Version: "v1"}
	restConfig.APIPath = "/api"
	restConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	o := &kubectlcp.CopyOptions{
		IOStreams: genericclioptions.IOStreams{
			Out:    os.Stdout,
			In:     os.Stdin,
			ErrOut: os.Stderr,
		},
		Namespace:    auth.KubeNamespace,
		Clientset:    kubeClient,
		ClientConfig: restConfig,
	}

	if len(srcSpec.PodName) != 0 {
		translatedSrcSpec, err := translateSpec(srcSpec, auth.AuthToken)
		if err != nil {
			return err
		}
		return o.CopyFromPod(translatedSrcSpec, destSpec)
	}
	if len(destSpec.PodName) != 0 {
		translatedDestSpec, err := translateSpec(destSpec, auth.AuthToken)
		if err != nil {
			return err
		}
		return o.CopyToPod(srcSpec, translatedDestSpec, &exec.ExecOptions{})
	}
	return errors.NewFriendlyError(
		"One of src or dest must be a remote file specification.")
}

func translateSpec(fileSpec kubectlcp.FileSpec, authToken string) (kubectlcp.FileSpec, error) {
	if len(fileSpec.PodNamespace) != 0 {
		return kubectlcp.FileSpec{}, errors.NewFriendlyError(
			"Specifying the remote namespace is not allowed.")
	}

	err := manager.CheckServiceRunning(fileSpec.PodName, authToken)
	if err != nil {
		return kubectlcp.FileSpec{}, err
	}

	return kubectlcp.FileSpec{
		PodName: kube.PodName(fileSpec.PodName),
		File:    fileSpec.File,
	}, nil
}
