package xprocessor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/mitchellh/mapstructure"
	"github.com/vjeantet/bitfan/api/models"
	"github.com/vjeantet/bitfan/codecs"
	"github.com/vjeantet/bitfan/processors"
)

func NewWithSpec(spec *models.XProcessor) processors.Processor {
	opt := &options{
		Behavior: spec.Behavior,
		Stream:   spec.Stream,
		Command:  spec.Command,
		Args:     spec.Args,
		Code:     spec.Code,
		StdinAs:  spec.StdinAs,
		StdoutAs: spec.StdoutAs,
	}

	if opt.Command == "php" && len(opt.Args) == 0 {
		if strings.HasPrefix(opt.Code, "<?php") {
			opt.Code = opt.Code[5:]
		}
		opt.Args = []string{"-d", "display_errors=stderr", "-r", opt.Code, "--"}
	}
	if opt.Command == "python" && len(opt.Args) == 0 {
		opt.Args = []string{"-u", "-c", opt.Code}
	}

	if spec.Stream == true {
		p := &streamProcessor{}
		p.opt = opt
		return p
	} else {
		p := &noStreamProcessor{}
		p.opt = opt
		return p
	}
}

const (
	TRANSFORMER string = "transformer"
	CONSUMER    string = "consumer"
	PRODUCER    string = "producer"
)

type options struct {
	processors.CommonOptions `mapstructure:",squash"`

	Codec codecs.CodecCollection

	// Producer ? Consumer ? Transformer ?
	// @Enum "producer","transformer","consumer"
	// @Default "transformer"
	Behavior string `mapstructure:"behavior" validate:"required"`

	// Delegated processor is started one time and receives events through its stdin.
	// When it should be started for each received event set value to "false"
	// @Default false
	Stream bool `mapstructure:"stream" `

	// Path to the bin used as delegated processor
	Command string   `mapstructure:"command" validate:"required"`
	Args    []string `mapstructure:"args" `
	Code    string   `mapstructure:"code"`

	// What is the value's format of stdinputed value
	// @Default "json","none"
	StdinAs string `mapstructure:"stdin_as" validate:"required"`

	// What is the value's format of stdoutputed value
	// @Enum "json","string"
	// @Default "json"
	StdoutAs string `mapstructure:"stdout_as" validate:"required"`

	// Flags for delegated processors (will be passed as args)
	// @default {"Content-Type" => "application/json"}
	Flags map[string]string
}

// Reads events from standard input
type processor struct {
	processors.Base

	opt *options

	wg *sync.WaitGroup
	q  chan bool
}

func (p *processor) Configure(ctx processors.ProcessorContext, conf map[string]interface{}) error {
	// remove common option from conf
	if err := mapstructure.WeakDecode(conf, &p.opt.CommonOptions); err != nil {
		return err
	}
	delete(conf, "add_field")
	delete(conf, "type")
	delete(conf, "remove_tag")
	delete(conf, "remove_field")
	delete(conf, "add_tag")
	delete(conf, "trace")
	delete(conf, "interval")

	// Set processor's user options
	if err := mapstructure.WeakDecode(conf, &p.opt.Flags); err != nil {
		return err
	}

	p.opt.Codec = codecs.CodecCollection{}
	if p.opt.StdinAs == "json" {
		p.opt.Codec.Enc = codecs.New("json", nil, ctx.Log(), ctx.ConfigWorkingLocation())
	}

	if p.opt.StdinAs == "line" {
		p.opt.Codec.Enc = codecs.New("line", map[string]interface{}{"format": "{{.message}}"}, ctx.Log(), ctx.ConfigWorkingLocation())
	}

	if p.opt.StdoutAs == "json" {
		p.opt.Codec.Dec = codecs.New("json", nil, ctx.Log(), ctx.ConfigWorkingLocation())
	}
	if p.opt.StdoutAs == "line" {
		p.opt.Codec.Dec = codecs.New("line", nil, ctx.Log(), ctx.ConfigWorkingLocation())
	}

	if p.opt.Behavior != TRANSFORMER && p.opt.Behavior != CONSUMER && p.opt.Behavior != PRODUCER {
		return fmt.Errorf("unknow behavior '%s'", p.opt.Behavior)
	}

	err := p.ConfigureAndValidate(ctx, conf, p.opt)
	if err != nil {
		return err
	}

	return err
}

func buildCommandArgs(args []string, flags map[string]string, e processors.IPacket) []string {
	finalArgs := []string{}
	for _, v := range args {
		finalArgs = append(finalArgs, v)
	}
	for k, v := range flags {
		if k == "_" {
			continue
		}
		if v == "" {
			finalArgs = append(finalArgs, "--"+k)
		} else {
			if e != nil {
				processors.Dynamic(&v, e.Fields())
			}
			finalArgs = append(finalArgs, "--"+k)
			finalArgs = append(finalArgs, v)
		}
	}
	if v, ok := flags["_"]; ok {
		finalArgs = append(finalArgs, v)
	}
	return finalArgs
}

func (p *processor) startCommand(e processors.IPacket) (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
	args := buildCommandArgs(p.opt.Args, p.opt.Flags, e)

	var cmd *exec.Cmd

	p.Logger.Debugf("command '%s', args=%s", p.opt.Command, args)
	cmd = exec.Command(p.opt.Command, args...)
	cmd.Dir = p.ConfigWorkingLocation
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("BF_PIPELINE_UUID=%s", p.PipelineUUID),
		fmt.Sprintf("BF_PIPELINE_WORKING_PATH=%s", p.ConfigWorkingLocation),
		fmt.Sprintf("BF_PROCESSOR_DATA_PATH=%s", p.DataLocation),
		fmt.Sprintf("BF_PROCESSOR_NAME=%s", p.B().Name),
		fmt.Sprintf("BF_PROCESSOR_LABEL=%s", p.B().Label),
	)

	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return cmd, stdin, stdout, stderr, err
	}

	p.Logger.Infof("delegator %s started PId=%d", p.B().Name, cmd.Process.Pid)
	return cmd, stdin, stdout, stderr, nil
}

func (p *processor) readAndSendEventsFromProcess(dec codecs.Decoder) error {
	defer p.wg.Done()
	for {
		var record interface{}
		if err := dec.Decode(&record); err != nil {
			if err == io.EOF {
				p.Logger.Debugf("codec end of file : %s", err.Error())
				break
			} else {
				p.Logger.Errorln("codec error : ", err.Error())
				return err
			}
		}

		var ne processors.IPacket
		switch v := record.(type) {
		case string:
			ne = p.NewPacket(map[string]interface{}{
				"message": v,
			})
		case map[string]interface{}:
			ne = p.NewPacket(v)
		case []interface{}:
			ne = p.NewPacket(map[string]interface{}{
				"data": v,
			})
		default:
			p.Logger.Errorf("Unknow structure %#v", v)
			continue
		}

		p.opt.ProcessCommonOptions(ne.Fields())
		p.Send(ne)
	}
	return nil
}