package text

// Icons resolves icon names to SVG markup for one render, and carries the render's math
// typesetter. Upstream keeps both in module variables for the length of a render; here
// they are passed down instead, so renders can run at the same time.
type Icons struct {
	resolve func(name string) string
	found   map[string]string
	math    *Math
}

// NewIcons returns the icons for a render. A nil resolve means the host has none.
func NewIcons(resolve func(name string) string) *Icons {
	return &Icons{resolve: resolve, found: map[string]string{}}
}

// Markup returns the host's markup for name, or "" when it has none. The host is asked once
// per name.
func (ic *Icons) Markup(name string) string {
	if ic == nil || ic.resolve == nil {
		return ""
	}
	markup, ok := ic.found[name]
	if !ok {
		markup = ic.resolve(name)
		ic.found[name] = markup
	}
	return markup
}

// Resolve returns the host's markup for name without remembering it, for drawing.
func (ic *Icons) Resolve(name string) string {
	if ic == nil || ic.resolve == nil {
		return ""
	}
	return ic.resolve(name)
}
