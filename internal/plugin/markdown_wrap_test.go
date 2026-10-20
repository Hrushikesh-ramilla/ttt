package plugin

import (
	"strings"
	"testing"

	"github.com/eugenioenko/ttt/internal/widgets"
)

func TestMarkdownWidgetWrapWidthInheritanceAndOverride(t *testing.T) {
	p := &Plugin{
		Name:    "mdtest",
		Granted: PermissionSet{PanelBottom: true},
	}
	p.Markdown.WrapWidth = 100
	p.State = NewSandbox()
	t.Cleanup(p.State.Close)
	setupTTTModule(p.State, p)

	err := p.State.DoString(`
		wrap_val = nil
		require("ttt").register({
			bottom = {
				title = "Test",
				render = function(panel)
					if wrap_val ~= nil then
						panel:markdown({ text = "Some long prose paragraph here.", wrap_width = wrap_val })
					else
						panel:markdown({ text = "Some long prose paragraph here." })
					end
				end,
			},
		})
	`)
	if err != nil {
		t.Fatal(err)
	}

	pw := NewPluginPanelWidget(p, p.BottomRenderFunc, nil)

	// 1. Inheritance: when wrap_width is omitted, inherits p.Markdown.WrapWidth (100)
	pw.Render(newMockSurface(120, 20))
	if len(pw.state.items) == 0 {
		t.Fatal("expected items in panel")
	}
	sv, ok := pw.state.items[0].(*widgets.ScrollViewWidget)
	if !ok {
		t.Fatalf("expected ScrollViewWidget, got %T", pw.state.items[0])
	}
	md, ok := sv.Child.(*widgets.MarkdownWidget)
	if !ok {
		t.Fatalf("expected MarkdownWidget, got %T", sv.Child)
	}
	if md.MaxWidth != 100 {
		t.Fatalf("expected inherited MaxWidth 100, got %d", md.MaxWidth)
	}

	// 2. Explicit override: wrap_width = 40
	err = p.State.DoString(`wrap_val = 40`)
	if err != nil {
		t.Fatal(err)
	}
	pw.Render(newMockSurface(120, 20))
	if md.MaxWidth != 40 {
		t.Fatalf("expected overridden MaxWidth 40, got %d", md.MaxWidth)
	}

	// 3. Updating the override: wrap_width = 60
	err = p.State.DoString(`wrap_val = 60`)
	if err != nil {
		t.Fatal(err)
	}
	pw.Render(newMockSurface(120, 20))
	if md.MaxWidth != 60 {
		t.Fatalf("expected updated MaxWidth 60, got %d", md.MaxWidth)
	}

	// 4. Removing the override: wrap_width = nil (omitted) -> returns to inherited 100
	err = p.State.DoString(`wrap_val = nil`)
	if err != nil {
		t.Fatal(err)
	}
	pw.Render(newMockSurface(120, 20))
	if md.MaxWidth != 100 {
		t.Fatalf("expected reverted MaxWidth 100, got %d", md.MaxWidth)
	}
}

func TestMarkdownWidgetWrapWidthValidation(t *testing.T) {
	tests := []struct {
		name    string
		luaCode string
	}{
		{
			name: "zero wrap_width",
			luaCode: `
				panel:markdown({ text = "hi", wrap_width = 0 })
			`,
		},
		{
			name: "negative wrap_width",
			luaCode: `
				panel:markdown({ text = "hi", wrap_width = -10 })
			`,
		},
		{
			name: "string wrap_width",
			luaCode: `
				panel:markdown({ text = "hi", wrap_width = "eighty" })
			`,
		},
		{
			name: "float wrap_width",
			luaCode: `
				panel:markdown({ text = "hi", wrap_width = 40.5 })
			`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &Plugin{
				Name:    "valtest",
				Granted: PermissionSet{PanelBottom: true},
			}
			p.State = NewSandbox()
			defer p.State.Close()
			setupTTTModule(p.State, p)

			code := `
				require("ttt").register({
					bottom = {
						title = "ValTest",
						render = function(panel)
			` + tc.luaCode + `
						end,
					},
				})
			`
			if err := p.State.DoString(code); err != nil {
				t.Fatal(err)
			}
			pw := NewPluginPanelWidget(p, p.BottomRenderFunc, nil)
			s := newMockSurface(80, 20)
			pw.Render(s)
			if p.LastError == nil {
				t.Fatalf("expected render error for invalid wrap_width %q", tc.name)
			}
		})
	}
}

func TestMarkdownWidgetWrapWidthLayout(t *testing.T) {
	// Verify that wrap_width limits prose line width, adapts to smaller widths,
	// and preserves non-wrapping blocks like code blocks.
	p := &Plugin{
		Name:    "layouttest",
		Granted: PermissionSet{PanelBottom: true},
	}
	p.State = NewSandbox()
	defer p.State.Close()
	setupTTTModule(p.State, p)

	longProse := "One two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty"
	codeBlock := "```\n" + strings.Repeat("A", 70) + "\n```"

	// 1. Prose-only verification: check that wrap_width=30 wraps prose to at most 30 columns
	err := p.State.DoString(`
		require("ttt").register({
			bottom = {
				title = "LayoutTest",
				render = function(panel)
					panel:markdown({
						text = [=[` + longProse + `]=],
						wrap_width = 30,
					})
				end,
			},
		})
	`)
	if err != nil {
		t.Fatal(err)
	}

	pw := NewPluginPanelWidget(p, p.BottomRenderFunc, nil)

	// Available width (100) > wrap_width (30): prose wraps within wrap_width
	pw.Render(newMockSurface(100, 30))
	sv := pw.state.items[0].(*widgets.ScrollViewWidget)
	md := sv.Child.(*widgets.MarkdownWidget)

	if md.MaxWidth != 30 {
		t.Fatalf("expected MaxWidth 30, got %d", md.MaxWidth)
	}
	wProseWide, _ := md.ScrollSize()
	if wProseWide > 30 {
		t.Fatalf("expected prose width <= 30 when available width is 100, got %d", wProseWide)
	}

	// Available width (20) < wrap_width (30): prose adapts to narrower surface
	pw.Render(newMockSurface(20, 30))
	wProseNarrow, _ := md.ScrollSize()
	if wProseNarrow > 20 {
		t.Fatalf("expected prose width <= 20 when available width is 20, got %d", wProseNarrow)
	}

	// 2. Code block overflow preservation: ensure unwrapped code block preserves width
	content := longProse + "\n\n" + codeBlock
	err = p.State.DoString(`
		require("ttt").register({
			bottom = {
				title = "LayoutTest",
				render = function(panel)
					panel:markdown({
						text = [=[` + content + `]=],
						wrap_width = 30,
					})
				end,
			},
		})
	`)
	if err != nil {
		t.Fatal(err)
	}

	pw = NewPluginPanelWidget(p, p.BottomRenderFunc, nil)
	pw.Render(newMockSurface(100, 30))
	sv = pw.state.items[0].(*widgets.ScrollViewWidget)
	md = sv.Child.(*widgets.MarkdownWidget)
	wCode, _ := md.ScrollSize()
	if wCode < 70 {
		t.Fatalf("code block width should be preserved, got w=%d", wCode)
	}
}
