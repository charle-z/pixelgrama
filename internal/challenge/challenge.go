package challenge

import "time"

const CatalogVersion = 1

type Daily struct {
	CatalogVersion int    `json:"catalog_version"`
	Date           string `json:"date"`
	Slug           string `json:"slug"`
	PromptES       string `json:"prompt_es"`
	PromptEN       string `json:"prompt_en"`
}

type entry struct {
	Slug     string
	PromptES string
	PromptEN string
}

var catalog = [...]entry{
	{Slug: "tiny-robot", PromptES: "Un robot diminuto", PromptEN: "A tiny robot"},
	{Slug: "moonlit-castle", PromptES: "Un castillo bajo la luna", PromptEN: "A castle under the moon"},
	{Slug: "sleepy-cat", PromptES: "Un gato con sueño", PromptEN: "A sleepy cat"},
	{Slug: "storm-cloud", PromptES: "Una nube de tormenta", PromptEN: "A storm cloud"},
	{Slug: "desert-cactus", PromptES: "Un cactus en el desierto", PromptEN: "A cactus in the desert"},
	{Slug: "arcade-spaceship", PromptES: "Una nave de arcade", PromptEN: "An arcade spaceship"},
	{Slug: "forest-mushroom", PromptES: "Un hongo del bosque", PromptEN: "A forest mushroom"},
	{Slug: "ocean-lighthouse", PromptES: "Un faro junto al océano", PromptEN: "A lighthouse by the ocean"},
	{Slug: "city-at-night", PromptES: "Una ciudad de noche", PromptEN: "A city at night"},
	{Slug: "friendly-ghost", PromptES: "Un fantasma amistoso", PromptEN: "A friendly ghost"},
	{Slug: "mountain-cabin", PromptES: "Una cabaña en la montaña", PromptEN: "A cabin in the mountains"},
	{Slug: "mechanical-flower", PromptES: "Una flor mecánica", PromptEN: "A mechanical flower"},
	{Slug: "cosmic-whale", PromptES: "Una ballena cósmica", PromptEN: "A cosmic whale"},
	{Slug: "rainy-window", PromptES: "Una ventana bajo la lluvia", PromptEN: "A window in the rain"},
	{Slug: "ancient-key", PromptES: "Una llave antigua", PromptEN: "An ancient key"},
	{Slug: "pocket-dragon", PromptES: "Un dragón de bolsillo", PromptEN: "A pocket dragon"},
}

func ForDate(value time.Time) Daily {
	utc := value.UTC()
	day := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	dayNumber := day.Unix() / int64((24*time.Hour)/time.Second)
	index := dayNumber % int64(len(catalog))
	if index < 0 {
		index += int64(len(catalog))
	}
	selected := catalog[index]
	return Daily{
		CatalogVersion: CatalogVersion,
		Date:           day.Format("2006-01-02"),
		Slug:           selected.Slug,
		PromptES:       selected.PromptES,
		PromptEN:       selected.PromptEN,
	}
}
