package workflow

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

var dutchIntakeCopy = map[string]string{
	"Enter an address, postcode or city. Your exact address stays private until you choose a professional.":                     "Voer een adres, postcode of plaats in. Je volledige adres blijft privé totdat je een vakprofessional kiest.",
	"Where is the problem, and what happens when you notice it?":                                                                "Waar is het probleem en wat merk je ervan?",
	"Does water still come out of the tap, and where is it leaking?":                                                            "Komt er nog water uit de kraan en waar lekt het?",
	"Is it one outlet or several parts of the property? Describe only what you've already noticed.":                             "Gaat het om één stopcontact of meerdere plekken in de woning? Beschrijf alleen wat je al hebt gemerkt.",
	"Is the door catching, or is the wood or frame damaged?":                                                                    "Klemt de deur of is het hout of kozijn beschadigd?",
	"When would you like someone to take a look?":                                                                               "Wanneer wil je dat iemand komt kijken?",
	"Do you have a budget in mind? It's okay if you're not sure yet.":                                                           "Heb je een budget in gedachten? Het is prima als je het nog niet weet.",
	"Where is the problem, and what happens when you notice it? A little more detail will help me find the right professional.": "Waar is het probleem en wat merk je ervan? Met wat meer informatie kan ik de juiste vakpersoon vinden.",
	"Which neighbourhood or postcode area in Amsterdam should the professional cover? Share a neighbourhood or postcode only; we’ll ask for the full address after you choose a professional.": "In welke buurt of welk postcodegebied in Amsterdam zoek je een vakpersoon? Deel alleen een buurt of postcode. We vragen het volledige adres nadat je een vakpersoon hebt gekozen.",
	"There isn’t an approved professional serving this area yet. You can save your request to the area waitlist. No visit has been booked.":                                                    "Er is nog geen goedgekeurde vakpersoon beschikbaar in dit gebied. Je kunt je aanvraag op de wachtlijst voor dit gebied zetten. Er is nog geen bezoek geboekt.",
	"Your request is on the area waitlist. Our team can review it, and no visit has been booked yet.":                                                                                          "Je aanvraag staat op de wachtlijst voor dit gebied. Ons team kan de aanvraag bekijken. Er is nog geen bezoek geboekt.",
	"Choose a professional to discuss your job and agree the plan, price and visit. Nothing is booked until you approve it.":                                                                   "Kies een vakpersoon om je klus te bespreken en het plan, de prijs en het bezoek af te spreken. Er is niets geboekt totdat je akkoord gaat.",
	"These professionals have no published visit times yet. Choose one and propose a time; the visit is booked when you both agree.":                                                           "Deze vakpersonen hebben nog geen bezoektijden gepubliceerd. Kies een vakpersoon en stel een tijd voor. Het bezoek is geboekt zodra jullie beiden akkoord zijn.",
	"I'll describe more":                 "Ik geef meer informatie",
	"I’ll describe more":                 "Ik geef meer informatie",
	"I'm not sure":                       "Ik weet het niet zeker",
	"Water runs · pipe drips underneath": "Het water loopt · de leiding drupt eronder",
	"No water comes out":                 "Er komt geen water uit",
	"The drain is backing up":            "Het water komt terug uit de afvoer",
	"One outlet":                         "Eén stopcontact",
	"Several rooms":                      "Meerdere kamers",
	"The door catches":                   "De deur klemt",
	"The wood or frame is damaged":       "Het hout of kozijn is beschadigd",
	"As soon as possible":                "Zo snel mogelijk",
	"In the next few days":               "In de komende dagen",
	"I'm flexible":                       "Ik ben flexibel",
	"I’m flexible":                       "Ik ben flexibel",
	"I’ll describe my budget":            "Ik beschrijf mijn budget",
	"Help me understand the cost":        "Help me de kosten te begrijpen",
}

func intakeText(locale, text string) string {
	if locale == "nl" {
		if translated, ok := dutchIntakeCopy[text]; ok {
			return translated
		}
		return text
	}
	for english, dutch := range dutchIntakeCopy {
		if text == dutch {
			return canonicalIntakeOption(english)
		}
	}
	return text
}

func canonicalIntakeOption(label string) string {
	switch label {
	case "I’ll describe more":
		return "I'll describe more"
	case "I’m flexible":
		return "I'm flexible"
	default:
		return label
	}
}

func intakeOptionID(step, label string) string {
	label = intakeText("en", label)
	known := map[string]string{
		"I'll describe more": "describe_more", "I'm not sure": "not_sure",
		"Water runs · pipe drips underneath": "water_runs_pipe_drips", "No water comes out": "no_water", "The drain is backing up": "drain_backs_up",
		"One outlet": "one_outlet", "Several rooms": "several_rooms", "The door catches": "door_catches", "The wood or frame is damaged": "damaged_frame",
		"As soon as possible": "asap", "In the next few days": "next_few_days", "I'm flexible": "flexible",
		"I’ll describe my budget": "describe_budget", "Help me understand the cost": "understand_cost",
	}
	key, ok := known[label]
	if !ok {
		key = fmt.Sprintf("%x", sha256.Sum256([]byte(label)))[:16]
	}
	return "intake:" + step + ":" + key
}

func intakeOptions(i *Intake, labels []string) {
	i.Options = make([]string, len(labels))
	i.OptionIDs = make([]string, len(labels))
	for index, label := range labels {
		i.Options[index] = intakeText(i.Locale, label)
		i.OptionIDs[index] = intakeOptionID(i.Step, label)
	}
}

func intakeLocale(requested, current string) string {
	if requested == "en" || requested == "nl" {
		return requested
	}
	if current == "nl" {
		return "nl"
	}
	return "en"
}

func intakeAnswerValue(i *Intake, value string) (string, string, bool) {
	if strings.HasPrefix(value, "intake:") {
		for index, label := range i.Options {
			if intakeOptionID(i.Step, label) == value {
				return intakeText("en", label), intakeText(i.Locale, label), true
			}
			if index < len(i.OptionIDs) && i.OptionIDs[index] == value {
				return intakeText("en", label), intakeText(i.Locale, label), true
			}
		}
		return "", "", false
	}
	for _, label := range i.Options {
		if strings.EqualFold(value, label) || strings.EqualFold(intakeText("en", value), intakeText("en", label)) {
			return intakeText("en", label), intakeText(i.Locale, label), true
		}
	}
	return value, value, true
}
