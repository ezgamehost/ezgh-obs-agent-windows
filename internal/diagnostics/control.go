package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"
)

type Reply struct {
	Status *Snapshot `json:"status,omitempty"`
	Error  string    `json:"error,omitempty"`
}

func (s *State) Serve(path string, reload func() error, doctor func() error) (func(), error) {
	l, e := listen(path)
	if e != nil {
		return nil, e
	}
	slots := make(chan struct{}, 8)
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			select {
			case slots <- struct{}{}:
				go func() {
					defer func() { <-slots }()
					defer c.Close()
					c.SetDeadline(time.Now().Add(3 * time.Second))
					var req struct {
						Op string `json:"op"`
					}
					if json.NewDecoder(io.LimitReader(c, 1024)).Decode(&req) != nil {
						return
					}
					var out Reply
					switch req.Op {
					case "status":
						v := s.Snapshot()
						out.Status = &v
					case "reload-credential":
						if reload() != nil {
							out.Error = "credential reload failed"
						}
					case "doctor":
						if doctor() != nil {
							out.Error = "service-context access check failed"
						} else {
							v := s.Snapshot()
							out.Status = &v
						}
					default:
						out.Error = "unsupported operation"
					}
					json.NewEncoder(c).Encode(out)
				}()
			default:
				c.Close()
			}
		}
	}()
	return func() { l.Close() }, nil
}
func Request(ctx context.Context, path, op string) (Reply, error) {
	c, e := dial(ctx, path)
	if e != nil {
		return Reply{}, errors.New("running agent control pipe unavailable")
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	if e = json.NewEncoder(c).Encode(map[string]string{"op": op}); e != nil {
		return Reply{}, e
	}
	var r Reply
	e = json.NewDecoder(io.LimitReader(c, 64<<10)).Decode(&r)
	if e == nil && r.Error != "" {
		e = errors.New(r.Error)
	}
	return r, e
}
