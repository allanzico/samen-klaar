package postgres

// Email always follows the recipient account's saved preference, independently
// of the sender's language or the language of the customer request.
func dutchNotificationSubject(event string) string {
	subjects := map[string]string{
		"coordination-question": "Een vervolgvraag over je afgesproken klus",
		"coordination-summary":  "Een toelichting op de klus staat klaar voor controle",
		"cancel-request":        "Update over de annulering van je aanvraag", "send-message": "Nieuw bericht over je aanvraag",
		"propose-price": "Nieuw prijsvoorstel voor je klus", "accept-price": "Prijsvoorstel geaccepteerd", "decline-price": "Prijsvoorstel afgewezen",
		"propose-date": "Nieuw voorstel voor een bezoek", "accept-date": "Voorgestelde bezoektijd geaccepteerd", "decline-date": "Voorgestelde bezoektijd afgewezen",
		"approve-and-book": "Update over je aangevraagde bezoek", "book-appointment": "Update over je aangevraagde bezoek", "confirm-appointment": "Je bezoek is bevestigd",
		"decline-appointment": "Kies een andere bezoektijd", "appointment-expired": "Kies een andere bezoektijd", "start-job": "Je bezoek is begonnen",
		"submit-evidence": "Het afgeronde werk staat klaar voor controle", "approve-completion": "Het afgeronde werk is goedgekeurd", "approve-payment": "Je betaalgoedkeuring is opgeslagen",
		"confirm-payment": "De betaling voor je klus is voltooid", "refund-completed": "De betaling voor je klus is terugbetaald", "payment-review": "De betaling voor je klus wordt nagekeken",
		"appointment-reminder": "Je bevestigde bezoek is binnen 24 uur", "missed-visit-reminder": "Controleer de status van je bezoek",
		"price-proposal-reminder": "Een prijsvoorstel wacht op je beslissing", "date-proposal-reminder": "Een bezoektijd wacht op je beslissing",
		"scope-change-reminder": "Een wijziging van de klus wacht op je beslissing", "evidence-overdue": "Het bewijs van afronding ontbreekt nog",
		"completion-note-overdue": "Voeg de toelichting bij het afgeronde werk toe", "completion-review-reminder": "Controleer het afgeronde werk",
		"payment-approval-reminder": "Controleer de betaling voor je afgeronde klus", "checkout-reminder": "Controleer de betaalstatus van je afgeronde klus",
		agentQuestionEvent: "Je agent heeft je beslissing nodig",
		"submit-review":    "Een klant heeft je afgeronde klus beoordeeld", "respond-to-review": "Je vakpersoon heeft op je beoordeling gereageerd",
	}
	if subject, ok := subjects[event]; ok {
		return subject
	}
	return "Update over je aanvraag"
}

func dutchNotificationBody(event, link string) string {
	opening := "Er is een update over een aanvraag in je SamenKlaar-account."
	switch event {
	case agentQuestionEvent:
		opening = "Je agent is gestopt bij een aanvraag, omdat de volgende stap jouw beslissing nodig heeft of buiten de grenzen valt die je hebt ingesteld. Open de aanvraag om de vraag te zien."
	case "appointment-reminder":
		opening = "Je bevestigde bezoek vindt binnen 24 uur plaats. Bekijk de bezoekgegevens in je account."
	case "missed-visit-reminder":
		opening = "De bevestigde bezoektijd is voorbij zonder dat de start is vastgelegd. Laat weten wat er is gebeurd, zodat we dit kunnen bekijken. Er is geen schuld vastgesteld."
	case "price-proposal-reminder":
		opening = "Een prijsvoorstel wacht al 24 uur op je beslissing. Bekijk het bedrag en accepteer het voorstel of wijs het af."
	case "date-proposal-reminder":
		opening = "Een voorgestelde bezoektijd wacht al 24 uur op je beslissing. Bekijk het voorstel en accepteer het of wijs het af."
	case "scope-change-reminder":
		opening = "Een voorgestelde wijziging van de klus wacht al 24 uur op je beslissing. De bestaande afspraken blijven gelden totdat je een wijziging accepteert."
	case "evidence-overdue":
		opening = "Het bezoek is voorbij en het bewijs van afronding ontbreekt nog. Upload het vereiste bewijs en dien het werk in ter controle."
	case "completion-note-overdue":
		opening = "De klant wacht op een controle van het afgeronde werk. Voeg de toelichting toe die nodig is om het ingediende bewijs te beoordelen."
	case "completion-review-reminder":
		opening = "Het bewijs van afronding staat klaar voor je controle. Keur het werk alleen goed als het overeenkomt met de afgesproken klus."
	case "payment-approval-reminder":
		opening = "Je afgeronde klus wacht op een aparte betaalbeslissing. PayPal schrijft het bedrag pas af nadat je dit hebt goedgekeurd."
	case "checkout-reminder":
		opening = "Je hebt het betaalbedrag goedgekeurd, maar de betaling is nog niet voltooid. Controleer eerst de opgeslagen PayPal-status. Laat een onzekere uitkomst nakijken voordat je opnieuw probeert."
	case "payment-review":
		opening = "PayPal heeft een betaalstatus gemeld die een medewerker moet nakijken. Open je aanvraag voor de actuele status."
	}
	return opening + "\n\nOpen je aanvraag: " + link + "\n\nJe ontvangt dit bericht omdat je een actieve aanvraag hebt."
}
