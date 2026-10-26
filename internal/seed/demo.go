package seed

import "context"

// Reserved test identities; these are fictional, not real trading professionals.
const demoCustomerID = "acct_alex-de-vries"
const demoCustomerSubject = "relay-customer-alex-de-vries"
const DemoCustomerName = "Alex de Vries"
const DemoCustomerEmail = "alex-de-vries@samenklaar.test"
const DemoProfessionalEmail = "noor-van-dijk@samenklaar.test"

// All candidates cover the same work in the same city, so price, conditions and
// availability can be compared without accidentally matching a different service.
var demoPros = []testPro{
	{slug: "noor-van-dijk", person: "Noor van Dijk", name: "Helder Kraanwerk", price: 9000, callout: 2000, materials: "Labour, one replacement PVC sink trap section and standard seals included. Other replacement parts need a separate customer-approved offer.", week: weeklyHours("09:00", "17:00", 1, 2, 3, 4, 5), mandate: mandate(90, true, true, 9, 17, []int{1, 2, 3, 4, 5}, true)},
	{slug: "milan-de-wit", person: "Milan de Wit", name: "Westerwater Loodgieters", price: 7500, callout: 0, materials: "Labour only; replacement parts are not included.", week: weeklyHours("09:00", "13:00", 2, 4), daysOffAhead: 4, mandate: mandate(100, true, true, 9, 13, []int{2, 4}, true)},
	{slug: "sara-jansen", person: "Sara Jansen", name: "Avondstroom Installaties", price: 10500, callout: 1500, materials: "Standard trap washer included; other replacement parts require approval.", week: weeklyHours("17:00", "21:00", 1, 2, 3, 4, 5), mandate: mandate(95, true, true, 17, 21, []int{1, 2, 3, 4, 5}, true)},
	{slug: "omar-bakker", person: "Omar Bakker", name: "Singel Sanitairservice", price: 9500, callout: 2500, materials: "Labour included; replacement parts quoted separately.", week: weeklyHours("10:00", "16:00", 6, 7), mandate: mandate(90, true, true, 10, 16, []int{6, 7}, true)},
	{slug: "lotte-meijer", person: "Lotte Meijer", name: "Kraan & Kompas", price: 12000, callout: 0, materials: "Standard washer and resealing consumables included.", week: weeklyHours("07:00", "12:00", 1, 3, 5), mandate: mandate(100, true, true, 7, 12, []int{1, 3, 5}, true)},
	{slug: "daan-vos", person: "Daan Vos", name: "Amstel Leidingwerk", price: 8500, callout: 2000, materials: "Labour only; customer approves any new parts before purchase.", week: weeklyHours("12:00", "18:00", 1, 2, 3, 4, 5), daysOffAhead: 7, mandate: mandate(85, true, true, 12, 18, []int{1, 2, 3, 4, 5}, true)},
	{slug: "priya-smit", person: "Priya Smit", name: "Waterlijn Techniek", price: 13000, callout: 1000, materials: "Standard washer included; full trap replacement is a separate offer.", week: weeklyHours("08:00", "20:00", 1, 2, 3, 4, 5, 6), mandate: mandate(95, true, true, 8, 20, []int{1, 2, 3, 4, 5, 6}, true)},
	{slug: "levi-bos", person: "Levi Bos", name: "De Gouden Sifon", price: 10000, callout: 500, materials: "Labour included; no replacement materials included.", week: weeklyHours("14:00", "18:00", 3, 5), mandate: mandate(90, false, false, 14, 18, []int{3, 5}, true)},
	{slug: "emma-vermeer", person: "Emma Vermeer", name: "Oever Loodgieters", price: 11000, callout: 2500, materials: "Standard trap washer included after inspection.", week: weeklyHours("09:00", "15:00", 1, 2, 3), daysOffAhead: 2, mandate: mandate(100, true, true, 9, 15, []int{1, 2, 3}, true)},
	{slug: "yusuf-de-boer", person: "Yusuf de Boer", name: "Kade & Kraan", price: 14500, callout: 1500, materials: "Labour and standard washer included; concealed repairs excluded.", week: weeklyHours("08:00", "18:00", 1, 2, 3, 4, 5), mandate: mandate(95, true, true, 8, 18, []int{1, 2, 3, 4, 5}, false)},
}

func demoProfessionals() []testPro {
	pros := append([]testPro(nil), demoPros...)
	for i := range pros {
		pros[i].locale, pros[i].category, pros[i].service = "en", "Plumbing", "Kitchen sink leak repair"
		pros[i].description = "Inspect and repair a leaking kitchen sink trap or accessible waste-pipe joint. Test the connection under running water."
		pros[i].area, pros[i].lat, pros[i].lon, pros[i].minutes = "Amsterdam", 52.37, 4.90, 60
		pros[i].exclusions = []string{"Concealed pipework", "Cabinet alterations", "Electrical work"}
	}
	return pros
}

// RunDemo prepares ten comparison candidates and one separate buyer. It does
// not connect wallets, create orders, approve funds or send notifications.
func RunDemo(ctx context.Context, databaseURL, issuer, password, output string) error {
	return runPros(ctx, databaseURL, issuer, password, output, demoProfessionals(), true)
}
