def normalize_tags(tags):
    normalized = sorted({str(tag).strip().lower() for tag in tags})
    return normalized if normalized else [""]
