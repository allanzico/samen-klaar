#!/usr/bin/env python3
"""Check the configured token with three public example locations; never print credentials."""
import json
import os
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode
from urllib.request import urlopen


def main():
    settings = {}
    env_file = Path(__file__).resolve().parent.parent / ".env"
    if env_file.exists():
        for line in env_file.read_text().splitlines():
            if "=" in line and not line.lstrip().startswith("#"):
                key, value = line.split("=", 1)
                settings[key.strip()] = value.strip().strip("\"'")
    token = os.environ.get("MAPBOX_ACCESS_TOKEN") or settings.get("MAPBOX_ACCESS_TOKEN", "")
    if not token:
        print("Set MAPBOX_ACCESS_TOKEN in .env or the environment first.")
        return 1
    for query, expected in [("Damrak 1 Amsterdam", "address"), ("1012 LG", "postcode"), ("Amsterdam", "place")]:
        parameters = urlencode({"q": query, "access_token": token, "country": "nl", "language": "en", "types": "address,postcode,place,locality,neighborhood", "limit": 5, "permanent": "true", "autocomplete": "false"})
        try:
            with urlopen("https://api.mapbox.com/search/geocode/v6/forward?" + parameters, timeout=10) as response:
                results = json.load(response).get("features", [])
        except HTTPError as error:
            print(f"Mapbox returned HTTP {error.code}. Check token access and permanent-geocoding billing.")
            return 1
        except (URLError, TimeoutError, ValueError):
            print("Mapbox could not be reached or returned an unreadable response.")
            return 1
        if not any(feature.get("properties", {}).get("feature_type") == expected and feature.get("properties", {}).get("context", {}).get("country", {}).get("country_code", "").lower() == "nl" for feature in results):
            print(f"No {expected} result for the public example: {query}")
            return 1
        print(f"OK: {expected} lookup with permanent geocoding ({query})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
