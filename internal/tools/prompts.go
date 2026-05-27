package tools

import "fmt"

func summarizeMapPrompt(focus string) string {
	p := "You are a precise summarizer. Summarize the following file contents concisely, preserving key facts, names, and structure. Output only the summary, with no preamble or conclusion."
	if focus != "" {
		p += " Focus especially on: " + focus + "."
	}
	return p
}

func summarizeReducePrompt() string {
	return "Combine the following partial summaries into one coherent, concise summary. Remove redundancy. Output only the summary, with no preamble."
}

func extractMapPrompt(query string) string {
	return fmt.Sprintf("Extract only the parts of the following file contents relevant to this query: %q. Return relevant snippets with their file path and no preamble. If nothing is relevant, say so briefly.", query)
}

func extractReducePrompt(query string) string {
	return fmt.Sprintf("Merge the following extracted findings into a single answer to the query: %q. Keep file references, remove duplicates, and include no preamble.", query)
}

func askMapPrompt(prompt string) string {
	return "You are a helpful assistant. Apply this instruction to the following file contents and output only the result, with no preamble.\n\nInstruction: " + prompt
}

func askReducePrompt(prompt string) string {
	return "Combine the following partial results into one coherent result for this instruction. Output only the result, with no preamble.\n\nInstruction: " + prompt
}

func askNoContextPrompt() string {
	return "You are a helpful assistant. Follow the instruction precisely and output only the result, with no preamble."
}
