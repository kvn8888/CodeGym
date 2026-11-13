def normalize_tags(tags):
    return sorted(str(tag).strip().lower() for tag in tags)
