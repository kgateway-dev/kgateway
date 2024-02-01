package ports

const portOffset = 8000

func TranslatePort(u uint16) uint16 {
	if u >= 1024 {
		return u
	}
	return u + portOffset
}
