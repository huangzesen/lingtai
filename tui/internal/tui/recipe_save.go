package tui

import (
	"os"
	"strings"
	"time"

	"github.com/anthropics/lingtai-tui/internal/fs"
	"github.com/anthropics/lingtai-tui/internal/preset"
)

// applyRecipe writes .prompt (from recipe's greet file with placeholder
// substitution) and .tui-asset/.recipe (recipe state tracking). Does NOT
// modify init.json — the caller sets AgentOpts.CommentFile before calling
// GenerateInitJSONWithOpts.
func applyRecipe(
	lingtaiDir, orchDir, globalDir, humanDir, humanAddr string,
	recipeName, customDir, lang, soulDelay string,
) error {
	var recipeDir string
	if recipeName == preset.RecipeCustom || recipeName == preset.RecipeImported {
		recipeDir = customDir
	} else {
		recipeDir = preset.RecipeDir(globalDir, recipeName)
	}

	greetPath := preset.ResolveGreetPath(recipeDir, lang)
	if greetPath != "" {
		data, err := os.ReadFile(greetPath)
		if err == nil {
			prompt := substituteGreetPlaceholders(string(data), humanAddr, humanDir, lang, soulDelay)
			fs.WritePrompt(orchDir, prompt)
		}
	}

	state := preset.RecipeState{Recipe: recipeName}
	if recipeName == preset.RecipeCustom || recipeName == preset.RecipeImported {
		state.CustomDir = customDir
	}
	return preset.SaveRecipeState(lingtaiDir, state)
}

// resolveRecipeComment returns the comment.md path for a recipe, for the
// caller to set on AgentOpts.CommentFile.
func resolveRecipeComment(globalDir, recipeName, customDir, lang string) string {
	var recipeDir string
	if recipeName == preset.RecipeCustom || recipeName == preset.RecipeImported {
		recipeDir = customDir
	} else {
		recipeDir = preset.RecipeDir(globalDir, recipeName)
	}
	return preset.ResolveCommentPath(recipeDir, lang)
}

// resolveRecipeCovenant returns the covenant.md path for a recipe, if the
// recipe provides one. Returns empty string if the recipe does not override
// the system-wide covenant.
func resolveRecipeCovenant(globalDir, recipeName, customDir, lang string) string {
	var recipeDir string
	if recipeName == preset.RecipeCustom || recipeName == preset.RecipeImported {
		recipeDir = customDir
	} else {
		recipeDir = preset.RecipeDir(globalDir, recipeName)
	}
	return preset.ResolveCovenantPath(recipeDir, lang)
}

// resolveRecipeProcedures returns the procedures.md path for a recipe, if the
// recipe provides one. Returns empty string if the recipe does not override
// the system-wide procedures.
func resolveRecipeProcedures(globalDir, recipeName, customDir, lang string) string {
	var recipeDir string
	if recipeName == preset.RecipeCustom || recipeName == preset.RecipeImported {
		recipeDir = customDir
	} else {
		recipeDir = preset.RecipeDir(globalDir, recipeName)
	}
	return preset.ResolveProceduresPath(recipeDir, lang)
}

// substituteGreetPlaceholders replaces canonical placeholder tokens in a greet
// template with runtime values before writing to .prompt.
func substituteGreetPlaceholders(template, humanAddr, humanDir, lang, soulDelay string) string {
	out := template
	out = strings.ReplaceAll(out, "{{time}}", time.Now().Format("2006-01-02 15:04"))
	out = strings.ReplaceAll(out, "{{addr}}", humanAddr)
	out = strings.ReplaceAll(out, "{{lang}}", lang)
	out = strings.ReplaceAll(out, "{{soul_delay}}", soulDelay)
	loc := "unknown"
	if humanDir != "" {
		if humanNode, err := fs.ReadAgent(humanDir); err == nil && humanNode.Location != nil {
			parts := []string{}
			if humanNode.Location.City != "" {
				parts = append(parts, humanNode.Location.City)
			}
			if humanNode.Location.Region != "" {
				parts = append(parts, humanNode.Location.Region)
			}
			if humanNode.Location.Country != "" {
				parts = append(parts, humanNode.Location.Country)
			}
			if len(parts) > 0 {
				loc = strings.Join(parts, ", ")
			}
		}
	}
	// If location is still unknown (first run, cache empty), try resolving
	// synchronously. ResolveLocation has a 5-second timeout built in.
	if loc == "unknown" {
		if resolved, err := fs.ResolveLocation(); err == nil {
			parts := []string{}
			if resolved.City != "" {
				parts = append(parts, resolved.City)
			}
			if resolved.Region != "" {
				parts = append(parts, resolved.Region)
			}
			if resolved.Country != "" {
				parts = append(parts, resolved.Country)
			}
			if len(parts) > 0 {
				loc = strings.Join(parts, ", ")
			}
			// Also persist it to human's .agent.json so next time it's cached
			if humanDir != "" {
				go fs.UpdateHumanLocation(humanDir)
			}
		}
	}
	out = strings.ReplaceAll(out, "{{location}}", loc)
	return out
}
