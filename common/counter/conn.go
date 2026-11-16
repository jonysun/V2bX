package counter

import (
	"io"
	"net"
	"sync"

	"github.com/sagernet/sing/common/bufio"

	"github.com/sagernet/sing/common/buf"

	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/network"
)

type ConnCounter struct {
	network.ExtendedConn
	storage   *TrafficStorage
	readFunc  network.CountFunc
	writeFunc network.CountFunc
	onClose   func()
	closeOnce sync.Once
	closeErr  error
}

func NewConnCounter(conn net.Conn, s *TrafficStorage) net.Conn {
	return NewConnCounterWithClose(conn, s, nil)
}

func NewConnCounterWithClose(conn net.Conn, s *TrafficStorage, onClose func()) net.Conn {
	return &ConnCounter{
		ExtendedConn: bufio.NewExtendedConn(conn),
		storage:      s,
		onClose:      onClose,
		readFunc: func(n int64) {
			s.UpCounter.Add(n)
		},
		writeFunc: func(n int64) {
			s.DownCounter.Add(n)
		},
	}
}

func (c *ConnCounter) Close() error {
	c.closeOnce.Do(func() {
		c.closeErr = c.ExtendedConn.Close()
		if c.onClose != nil {
			c.onClose()
		}
	})
	return c.closeErr
}

func (c *ConnCounter) Read(b []byte) (n int, err error) {
	n, err = c.ExtendedConn.Read(b)
	c.storage.UpCounter.Add(int64(n))
	return
}

func (c *ConnCounter) Write(b []byte) (n int, err error) {
	n, err = c.ExtendedConn.Write(b)
	c.storage.DownCounter.Add(int64(n))
	return
}

func (c *ConnCounter) ReadBuffer(buffer *buf.Buffer) error {
	err := c.ExtendedConn.ReadBuffer(buffer)
	if err != nil {
		return err
	}
	if buffer.Len() > 0 {
		c.storage.UpCounter.Add(int64(buffer.Len()))
	}
	return nil
}

func (c *ConnCounter) WriteBuffer(buffer *buf.Buffer) error {
	dataLen := int64(buffer.Len())
	err := c.ExtendedConn.WriteBuffer(buffer)
	if err != nil {
		return err
	}
	if dataLen > 0 {
		c.storage.DownCounter.Add(dataLen)
	}
	return nil
}

func (c *ConnCounter) UnwrapReader() (io.Reader, []network.CountFunc) {
	return c.ExtendedConn, []network.CountFunc{
		c.readFunc,
	}
}

func (c *ConnCounter) UnwrapWriter() (io.Writer, []network.CountFunc) {
	return c.ExtendedConn, []network.CountFunc{
		c.writeFunc,
	}
}

func (c *ConnCounter) Upstream() any {
	return c.ExtendedConn
}

type PacketConnCounter struct {
	network.PacketConn
	storage   *TrafficStorage
	readFunc  network.CountFunc
	writeFunc network.CountFunc
	onClose   func()
	closeOnce sync.Once
	closeErr  error
}

func NewPacketConnCounter(conn network.PacketConn, s *TrafficStorage) network.PacketConn {
	return NewPacketConnCounterWithClose(conn, s, nil)
}

func NewPacketConnCounterWithClose(conn network.PacketConn, s *TrafficStorage, onClose func()) network.PacketConn {
	return &PacketConnCounter{
		PacketConn: conn,
		storage:    s,
		onClose:    onClose,
		readFunc: func(n int64) {
			s.UpCounter.Add(n)
		},
		writeFunc: func(n int64) {
			s.DownCounter.Add(n)
		},
	}
}

func (p *PacketConnCounter) Close() error {
	p.closeOnce.Do(func() {
		p.closeErr = p.PacketConn.Close()
		if p.onClose != nil {
			p.onClose()
		}
	})
	return p.closeErr
}

func (p *PacketConnCounter) ReadPacket(buff *buf.Buffer) (destination M.Socksaddr, err error) {
	destination, err = p.PacketConn.ReadPacket(buff)
	if err != nil {
		return
	}
	p.storage.UpCounter.Add(int64(buff.Len()))
	return
}

func (p *PacketConnCounter) WritePacket(buff *buf.Buffer, destination M.Socksaddr) (err error) {
	n := buff.Len()
	err = p.PacketConn.WritePacket(buff, destination)
	if err != nil {
		return
	}
	if n > 0 {
		p.storage.DownCounter.Add(int64(n))
	}
	return
}

func (p *PacketConnCounter) UnwrapPacketReader() (network.PacketReader, []network.CountFunc) {
	return p.PacketConn, []network.CountFunc{
		p.readFunc,
	}
}

func (p *PacketConnCounter) UnwrapPacketWriter() (network.PacketWriter, []network.CountFunc) {
	return p.PacketConn, []network.CountFunc{
		p.writeFunc,
	}
}

func (p *PacketConnCounter) Upstream() any {
	return p.PacketConn
}
