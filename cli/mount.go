package cli

import (
	"context"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/uor-framework/uor-client-go/attributes/matchers"
	"github.com/uor-framework/uor-client-go/config"
	"github.com/uor-framework/uor-client-go/registryclient/orasclient"
	"github.com/winfsp/cgofuse/fuse"
	"os"
	"os/signal"
	"syscall"
)

// MountOptions describe configuration options that can
// be set using the pull subcommand.
type MountOptions struct {
	*RootOptions
	Source         string
	MountPoint     string
	Insecure       bool
	MountAll       bool
	PlainHTTP      bool
	Configs        []string
	AttributeQuery string
	NoVerify       bool
}

//var clientMountExamples = []examples.Example{
//	{
//		RootCommand:   filepath.Base(os.Args[0]),
//		CommandString: "mount localhost:5001/test:latest ./mount-dir/",
//		Descriptions: []string{
//			"Mount collection reference.",
//		},
//	},
//	{
//		RootCommand:   filepath.Base(os.Args[0]),
//		CommandString: "mount localhost:5001/test:latest --pull-all ./mount-dir/",
//		Descriptions: []string{
//			"Mount collection reference with all linked references.",
//		},
//	},
//}

// NewMountCmd creates a new cobra.Command for the mount subcommand.
// TODO decide whether to use traditional mount -o flag format or to reuse uor-client-go flags
func NewMountCmd(rootOpts *RootOptions) *cobra.Command {
	o := MountOptions{RootOptions: rootOpts}

	cmd := &cobra.Command{
		Use:   "mount [flags] SRC MOUNTPOINT",
		Short: "Mount a UOR collection based on content or attribute address",
		//Example:       examples.FormatExamples(clientMountExamples...),
		SilenceErrors: false,
		SilenceUsage:  false,
		Args:          cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			cobra.CheckErr(o.Complete(args))
			cobra.CheckErr(o.Validate())
			cobra.CheckErr(o.Run(cmd.Context()))
		},
	}

	cmd.Flags().StringArrayVarP(&o.Configs, "configs", "c", o.Configs, "auth config paths when contacting registries")
	cmd.Flags().BoolVarP(&o.Insecure, "insecure", "i", o.Insecure, "allow connections to SSL registry without certs")
	cmd.Flags().BoolVar(&o.PlainHTTP, "plain-http", o.PlainHTTP, "use plain http and not https when contacting registries")
	cmd.Flags().StringVarP(&o.MountPoint, "output", "o", o.MountPoint, "output location for artifacts")
	cmd.Flags().StringVar(&o.AttributeQuery, "attributes", o.AttributeQuery, "attribute query config path")
	cmd.Flags().BoolVar(&o.MountAll, "pull-all", o.MountAll, "pull all linked collections")
	cmd.Flags().BoolVarP(&o.NoVerify, "no-verify", "", o.NoVerify, "skip collection signature verification")

	return cmd
}

func (o *MountOptions) Complete(args []string) error {
	if len(args) < 2 {
		return errors.New("bug: expecting one argument")
	}
	o.Source = args[0]
	o.MountPoint = args[1]
	return nil
}

func (o *MountOptions) Validate() error {
	mountPointStat, err := os.Stat(o.MountPoint)
	if err != nil {
		return err
	}
	if !mountPointStat.IsDir() {
		return errors.New("mount point must be a directory")
	}
	return nil
}

//func (o *MountOptions) Run(ctx context.Context) error {
//	return nil
//}

//func (uorFs *UorFs) Mount(mountpoint string) bool {
//	//uorFs := &UorFs{}
//	host := fuse.NewFileSystemHost(uorFs)
//	host.SetCapReaddirPlus(true)
//	opts := []string{}
//	opts = append([]string{mountpoint}, opts...)
//	cgofuseMountResult := host.Mount("", opts)
//	return cgofuseMountResult
//}

func unmountOnInterrupt(host *fuse.FileSystemHost) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(
		interrupt,
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	<-interrupt
	host.Unmount()
}

func (o *MountOptions) Run(ctx context.Context) error {

	o.Logger.Infof("Resolving artifacts for reference %s", o.Source)
	matcher := matchers.PartialAttributeMatcher{}
	if o.AttributeQuery != "" {
		query, err := config.ReadAttributeQuery(o.AttributeQuery)
		if err != nil {
			return err
		}

		attributeSet, err := config.ConvertToModel(query.Attributes)
		if err != nil {
			return err
		}
		matcher = attributeSet.List()
	}

	if !o.NoVerify {
		o.Logger.Infof("Checking signature of %s", o.Source)
		//if err := verifyCollection(o, ctx); err != nil {
		//	return err
		//}

	}

	client, err := orasclient.NewClient(
		orasclient.SkipTLSVerify(o.Insecure),
		orasclient.WithAuthConfigs(o.Configs),
		orasclient.WithPlainHTTP(o.PlainHTTP),
		orasclient.WithPullableAttributes(matcher),
	)
	if err != nil {
		return fmt.Errorf("error configuring client: %v", err)
	}

	fuseHost := fuse.NewFileSystemHost(NewUorFs(ctx, *o, client, matcher))
	fuseHost.SetCapReaddirPlus(true)
	go unmountOnInterrupt(fuseHost)
	o.Logger.Infof("Mounting UOR to directory %v", o.MountPoint)
	opts := []string{
		"-o", "fsname=uorfs",
		"-o", "ro",
		"-o", "default_permissions",
		"-o", "auto_unmount",
		//"-o", "user_xattr",
	}
	mounted := fuseHost.Mount(o.MountPoint, opts)
	o.Logger.Infof("%v", mounted)

	return nil
}
