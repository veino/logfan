package xprocessor

import (
	"bufio"
	"io"
	"os/exec"
	"sync"

	"github.com/vjeantet/bitfan/codecs"
	"github.com/vjeantet/bitfan/processors"
)

// Reads events from standard input
type streamProcessor struct {
	processor

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
}

func (p *streamProcessor) Start(e processors.IPacket) error {
	p.wg = new(sync.WaitGroup)

	p.Logger.Infof("Start %s %s", p.opt.Behavior, p.opt.Stream)
	var err error
	p.cmd, p.stdin, p.stdout, p.stderr, err = p.startCommand(nil)

	go func(s io.ReadCloser) {
		scanner := bufio.NewScanner(s)
		for scanner.Scan() {
			p.Logger.Errorf("stderr : %s", scanner.Text())
		}
	}(p.stderr)

	if p.opt.Behavior == PRODUCER || p.opt.Behavior == TRANSFORMER {
		var dec codecs.Decoder
		if dec, err = p.opt.Codec.NewDecoder(p.stdout); err != nil {
			p.Logger.Errorln("decoder error : ", err.Error())
			return err
		}
		// READ FROM PROC OUTPUT AND SEND EVENTS
		p.wg.Add(2)
		go func(s io.ReadCloser) {
			defer p.wg.Done()
			fscanner := bufio.NewScanner(s)
			for fscanner.Scan() {
				p.Logger.Errorf("stderr : %s", fscanner.Text())
			}
		}(p.stderr)
		go p.readAndSendEventsFromProcess(dec)
	}

	return nil
}

func (p *streamProcessor) Tick(e processors.IPacket) error {
	return nil
}

func (p *streamProcessor) Receive(e processors.IPacket) error {
	var err error
	// Encode received event
	var enc codecs.Encoder
	enc, err = p.opt.Codec.NewEncoder(p.stdin)
	if err != nil {
		p.Logger.Errorln("codec error : ", err.Error())
		return err
	}
	enc.Encode(e.Fields().Old())
	p.stdin.Write([]byte{'\n'})

	return nil
}

func (p *streamProcessor) Stop(e processors.IPacket) error {
	if p.cmd != nil {
		p.stdin.Close()
		// p.cmd.Process.Signal(syscall.SIGQUIT)
		p.cmd.Wait()
	}
	return nil
}