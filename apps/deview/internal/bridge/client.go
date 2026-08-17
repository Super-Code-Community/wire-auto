package bridge

// Client — тонкая обёртка над Transport с операциями протокола моста.
type Client struct{ t Transport }

func NewClient(t Transport) *Client { return &Client{t: t} }

// List шлёт list и читает события до catalog.
func (c *Client) List() ([]Script, error) {
	if err := c.t.Send(Command{Type: "list"}); err != nil {
		return nil, err
	}
	for {
		e, err := c.t.Recv()
		if err != nil {
			return nil, err
		}
		if e.Type == "catalog" {
			return e.Scripts, nil
		}
		// прочие события до catalog игнорируем (в v1 их нет)
	}
}

// Run шлёт run{dir}, вызывает onEvent на ready/log и возвращает терминальное
// событие (result или error). Ошибку возвращает лишь при обрыве транспорта.
func (c *Client) Run(dir string, onEvent func(Event)) (Event, error) {
	if err := c.t.Send(Command{Type: "run", Dir: dir}); err != nil {
		return Event{}, err
	}
	for {
		e, err := c.t.Recv()
		if err != nil {
			return Event{}, err
		}
		switch e.Type {
		case "ready", "log":
			if onEvent != nil {
				onEvent(e)
			}
		case "result", "error":
			return e, nil
		}
	}
}

// Cancel просит мост отменить текущий прогон.
func (c *Client) Cancel() error { return c.t.Send(Command{Type: "cancel"}) }

// Close завершает мост и освобождает ресурсы транспорта.
func (c *Client) Close() error { return c.t.Close() }
