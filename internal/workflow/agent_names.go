package workflow

import (
	"hash/fnv"
	"strings"
)

// Assistants have short, friendly bot names instead of "<person>'s assistant". The name is stable for
// the same person, and the two assistants on one request never share a name.
var assistantNames = []string{"Pip", "Juno", "Kit", "Remy", "Nova", "Finn", "Mika", "Luca", "Noor", "Teo", "Ivy", "Sky", "Rio", "Ada", "Zed", "Bo"}

func assistantName(seed, avoid string) string {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(strings.ToLower(strings.TrimSpace(seed))))
	index := int(hash.Sum32() % uint32(len(assistantNames)))
	if assistantNames[index] == avoid {
		index = (index + 1) % len(assistantNames)
	}
	return assistantNames[index]
}

// AssistantNames are the names of the customer's and the professional's assistants on a request.
func AssistantNames(job Job) (customer, professional string) {
	customer = assistantName(job.Customer, "")
	seed := job.ProfessionalAccountID
	if seed == "" {
		seed = job.Professional
	}
	return customer, assistantName(seed, customer)
}

// AssistantName is the name of the assistant acting for the given role ("customer" or "professional").
func AssistantName(job Job, role string) string {
	customer, professional := AssistantNames(job)
	if role == "professional" {
		return professional
	}
	return customer
}
