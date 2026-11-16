package rate

import (
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/network"
)

type PacketConn struct {
	network.PacketConn
	limiter Waiter
}

func NewPacketConnRateLimiter(c network.PacketConn, l Waiter) network.PacketConn {
	return &PacketConn{PacketConn: c, limiter: l}
}

func (c *PacketConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	dest, err := c.PacketConn.ReadPacket(buffer)
	if err == nil {
		c.limiter.Wait(int64(buffer.Len()))
	}
	return dest, err
}

func (c *PacketConn) WritePacket(buffer *buf.Buffer, dest M.Socksaddr) error {
	c.limiter.Wait(int64(buffer.Len()))
	return c.PacketConn.WritePacket(buffer, dest)
}
