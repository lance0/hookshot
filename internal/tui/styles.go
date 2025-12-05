package tui

import "github.com/charmbracelet/lipgloss"

// Catppuccin Mocha color palette
var (
	// Base colors
	Base     = lipgloss.Color("#1E1E2E")
	Mantle   = lipgloss.Color("#181825")
	Crust    = lipgloss.Color("#11111B")
	Surface0 = lipgloss.Color("#313244")
	Surface1 = lipgloss.Color("#45475A")
	Surface2 = lipgloss.Color("#585B70")

	// Text colors
	Text     = lipgloss.Color("#CDD6F4")
	Subtext1 = lipgloss.Color("#BAC2DE")
	Subtext0 = lipgloss.Color("#A6ADC8")
	Overlay2 = lipgloss.Color("#9399B2")
	Overlay1 = lipgloss.Color("#7F849C")
	Overlay0 = lipgloss.Color("#6C7086")

	// Accent colors
	Rosewater = lipgloss.Color("#F5E0DC")
	Flamingo  = lipgloss.Color("#F2CDCD")
	Pink      = lipgloss.Color("#F5C2E7")
	Mauve     = lipgloss.Color("#CBA6F7")
	Red       = lipgloss.Color("#F38BA8")
	Maroon    = lipgloss.Color("#EBA0AC")
	Peach     = lipgloss.Color("#FAB387")
	Yellow    = lipgloss.Color("#F9E2AF")
	Green     = lipgloss.Color("#A6E3A1")
	Teal      = lipgloss.Color("#94E2D5")
	Sky       = lipgloss.Color("#89DCEB")
	Sapphire  = lipgloss.Color("#74C7EC")
	Blue      = lipgloss.Color("#89B4FA")
	Lavender  = lipgloss.Color("#B4BEFE")
)

// Method colors
var MethodColors = map[string]lipgloss.Color{
	"GET":     Green,
	"POST":    Peach,
	"PUT":     Blue,
	"DELETE":  Red,
	"PATCH":   Mauve,
	"OPTIONS": Teal,
	"HEAD":    Overlay1,
}

// Status code colors
func StatusColor(code int) lipgloss.Color {
	switch {
	case code >= 500:
		return Red
	case code >= 400:
		return Yellow
	case code >= 300:
		return Sky
	case code >= 200:
		return Green
	default:
		return Overlay0
	}
}

// Styles
var (
	// Title bar
	TitleStyle = lipgloss.NewStyle().
			Foreground(Mauve).
			Bold(true)

	// Box styles
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Lavender).
			Padding(0, 1)

	HeaderBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Lavender).
			Padding(0, 1).
			BorderBottom(false)

	ListBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Lavender).
			Padding(0, 1).
			BorderTop(false).
			BorderBottom(false)

	DetailBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Lavender).
			Padding(0, 1).
			BorderTop(false)

	// Selected row
	SelectedStyle = lipgloss.NewStyle().
			Background(Surface0).
			Foreground(Text)

	// Normal row
	NormalStyle = lipgloss.NewStyle().
			Foreground(Subtext0)

	// Dim text
	DimStyle = lipgloss.NewStyle().
			Foreground(Overlay0)

	// Section header
	SectionStyle = lipgloss.NewStyle().
			Foreground(Lavender).
			Bold(true)

	// Help text
	HelpStyle = lipgloss.NewStyle().
			Foreground(Overlay0)

	// URL style
	URLStyle = lipgloss.NewStyle().
			Foreground(Sky).
			Underline(true)

	// Success indicator
	SuccessStyle = lipgloss.NewStyle().
			Foreground(Green)

	// Error indicator
	ErrorStyle = lipgloss.NewStyle().
			Foreground(Red)

	// Emoji/icon style
	IconStyle = lipgloss.NewStyle().
			Foreground(Mauve)
)

// MethodStyle returns the style for a given HTTP method
func MethodStyle(method string) lipgloss.Style {
	color, ok := MethodColors[method]
	if !ok {
		color = Overlay1
	}
	return lipgloss.NewStyle().Foreground(color).Bold(true)
}

// StatusStyle returns the style for a given status code
func StatusStyle(code int) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(StatusColor(code))
}
