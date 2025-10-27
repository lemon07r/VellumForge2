package config

// GetDefaultSubtopicTemplate returns the default template for subtopic generation
func GetDefaultSubtopicTemplate() string {
	return `You are a creative writing expert specializing in fantasy fiction. Your task is to generate {{.NumSubtopics}} distinct and imaginative subtopics for the main theme: "{{.MainTopic}}".

Each subtopic should be:
- Specific and focused enough to inspire detailed story prompts
- Unique from the others in the list
- Rich with potential for creative exploration
- Grounded in the fantasy genre

Return ONLY a valid JSON array of strings (no markdown, no additional text):
["Subtopic 1", "Subtopic 2", ...]`
}

// GetDefaultPromptTemplate returns the default template for prompt generation
func GetDefaultPromptTemplate() string {
	return `You are a creative writing prompt generator specializing in fantasy fiction. Generate {{.NumPrompts}} unique and compelling story prompts for the subtopic: "{{.SubTopic}}".

Each prompt should:
- Be detailed enough to inspire a complete short story (2-4 sentences)
- Include specific characters, settings, or situations
- Have inherent conflict or tension
- Be suitable for fantasy fiction writing
- Be distinct from the other prompts

Return ONLY a valid JSON array of strings (no markdown, no additional text):
["Prompt 1", "Prompt 2", ...]`
}

// GetDefaultJudgeTemplate returns the default template for LLM-as-a-Judge evaluation
// This is a simplified version - the full rubric would be embedded here
func GetDefaultJudgeTemplate() string {
	return `You are an expert literary editor and judge for a prestigious fantasy fiction award. Your task is to evaluate the following story based on a detailed 12-point rubric.

STORY TO EVALUATE:
{{.StoryText}}

EVALUATION RUBRIC:
For each of the 12 criteria below, provide:
1. A "reasoning" paragraph (2-3 sentences) explaining your analysis
2. A "score" from 1 to 5, where:
   - 1 = Nascent (fundamental flaws)
   - 2 = Developing (significant issues)
   - 3 = Competent (functional but basic)
   - 4 = Proficient (strong execution)
   - 5 = Masterful (exceptional quality)

The 12 criteria are:
1. plot_and_structural_integrity
2. character_and_dialogue
3. world_building_and_immersion
4. prose_style_and_voice
5. stylistic_and_lexical_slop
6. narrative_formula_and_archetypal_simplicity
7. coherence_and_factual_consistency
8. content_generation_vs_evasion
9. nuanced_portrayal_of_sensitive_themes
10. grammatical_and_syntactical_accuracy
11. clarity_conciseness_and_word_choice
12. structural_and_paragraphical_organization

Return ONLY a valid JSON object with this exact structure (no markdown, no additional text):
{
  "plot_and_structural_integrity": {
    "score": <1-5>,
    "reasoning": "<your analysis>"
  },
  "character_and_dialogue": {
    "score": <1-5>,
    "reasoning": "<your analysis>"
  },
  ... (continue for all 12 criteria)
}

IMPORTANT: Your response must be valid JSON and nothing else.`
}
