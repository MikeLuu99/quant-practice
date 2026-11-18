package games

import (
	"encoding/json"

	"quantgame/backend/internal/engine"
)

// Registry: maps API slugs to engine constructors.
func init() {
	engine.Register("assessment", newAssessment)
	engine.Register("trainer", newTrainer)
	engine.Register("fruit", func(cfg json.RawMessage, ctx *engine.Ctx) (engine.Game, error) { return newFruit(cfg, ctx) })
	engine.Register("math", func(cfg json.RawMessage, ctx *engine.Ctx) (engine.Game, error) { return newMath(cfg, ctx) })
	engine.Register("cards", func(cfg json.RawMessage, ctx *engine.Ctx) (engine.Game, error) { return newCards(cfg, ctx) })
	engine.Register("marble", func(cfg json.RawMessage, ctx *engine.Ctx) (engine.Game, error) { return newMarble(cfg, ctx) })
	engine.Register("etf", func(cfg json.RawMessage, ctx *engine.Ctx) (engine.Game, error) { return newETF(cfg, ctx) })
	engine.Register("nextcard", func(cfg json.RawMessage, ctx *engine.Ctx) (engine.Game, error) { return newNextCard(cfg, ctx) })
	engine.Register("prob", func(cfg json.RawMessage, ctx *engine.Ctx) (engine.Game, error) { return newProb(cfg, ctx) })
	engine.Register("fermi", func(cfg json.RawMessage, ctx *engine.Ctx) (engine.Game, error) { return newFermi(cfg, ctx) })
	engine.Register("makeme", func(cfg json.RawMessage, ctx *engine.Ctx) (engine.Game, error) { return newMakeMe(cfg, ctx) })
}
