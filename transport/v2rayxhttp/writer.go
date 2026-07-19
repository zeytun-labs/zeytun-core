package xhttp

import (
	common "github.com/sagernet/sing-box/common/xray"
	"github.com/sagernet/sing-box/common/xray/buf"
	"github.com/sagernet/sing-box/common/xray/pipe"
)

type uploadWriter struct {
	*pipe.Writer
	maxLen int32
}

func (w uploadWriter) Write(b []byte) (int, error) {
	buffer := buf.MultiBufferContainer{}
	common.Must2(buffer.Write(b))
	var writed int
	for _, buff := range buffer.MultiBuffer {
		err := w.WriteMultiBuffer(buf.MultiBuffer{buff})
		if err != nil {
			return writed, err
		}
		writed += int(buff.Len())
	}
	return writed, nil
}
