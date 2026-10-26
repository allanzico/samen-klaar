# Test professionals

For the ten same-service Amsterdam professionals and the customer used in the presentation, see [the presentation plan](HACKATHON_DEMO.md). That separate cohort uses `seed-demo` and `import-seed-demo-auth0`; it preserves the mixed-trade accounts below.

The local-only `make seed-pros` tool creates these accounts and initializes weekly working hours, days off and generated availability. It never creates one-off free times. Existing profiles, weekly hours, days off and assistant preferences survive repeat runs.

The set includes weekday, evening, weekend and limited weekly availability. Omar starts with 25 days off; Kwame has no published working hours and a paused assistant. Professionals can edit these schedules through the normal UI. This change does not run the seed tool against production or alter existing accounts there.

```text
jane-price@samenklaar.test
mike-dawson@samenklaar.test
sanne-de-vries@samenklaar.test
omar-haddad@samenklaar.test
lotte-jansen@samenklaar.test
tom-whitaker@samenklaar.test
priya-nair@samenklaar.test
daan-visser@samenklaar.test
chloe-martin@samenklaar.test
kwame-boateng@samenklaar.test
```
