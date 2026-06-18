#!/usr/bin/env python3
"""Manage the CodeGym GitHub Projects v2 kanban board.

This script intentionally avoids the `gh` CLI so agents can use it in Codex
environments where GitHub's project-column tools are not exposed. It needs a
token with project scope in GH_TOKEN or GITHUB_TOKEN.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import textwrap
import urllib.error
import urllib.request
from dataclasses import dataclass
from typing import Any

API_URL = "https://api.github.com/graphql"
DEFAULT_OWNER = "kvn8888"
DEFAULT_REPO = "CodeGym"
DEFAULT_PROJECT_TITLE = "CodeGym"


class GitHubError(RuntimeError):
    pass


@dataclass(frozen=True)
class ProjectRef:
    id: str
    number: int
    title: str
    url: str
    fields: list[dict[str, Any]]


class GitHubClient:
    def __init__(self, token: str) -> None:
        self.token = token

    def graphql(self, query: str, variables: dict[str, Any] | None = None) -> dict[str, Any]:
        body = json.dumps({"query": query, "variables": variables or {}}).encode()
        req = urllib.request.Request(
            API_URL,
            data=body,
            headers={
                "Authorization": f"Bearer {self.token}",
                "Content-Type": "application/json",
                "Accept": "application/vnd.github+json",
                "User-Agent": "codegym-project-board-script",
            },
            method="POST",
        )
        try:
            with urllib.request.urlopen(req, timeout=30) as res:
                payload = json.loads(res.read().decode())
        except urllib.error.HTTPError as err:
            detail = err.read().decode(errors="replace")
            raise GitHubError(f"GitHub API HTTP {err.code}: {detail}") from err
        except urllib.error.URLError as err:
            raise GitHubError(f"GitHub API request failed: {err}") from err

        if payload.get("errors"):
            messages = "; ".join(error.get("message", str(error)) for error in payload["errors"])
            raise GitHubError(messages)
        return payload["data"]


USER_PROJECTS_QUERY = """
query UserProjects($login: String!, $first: Int!) {
  user(login: $login) {
    projectsV2(first: $first, orderBy: {field: UPDATED_AT, direction: DESC}) {
      nodes {
        id
        number
        title
        url
        fields(first: 50) {
          nodes {
            __typename
            ... on ProjectV2FieldCommon {
              id
              name
              dataType
            }
            ... on ProjectV2SingleSelectField {
              id
              name
              dataType
              options { id name }
            }
          }
        }
      }
    }
  }
}
"""

ORG_PROJECTS_QUERY = """
query OrgProjects($login: String!, $first: Int!) {
  organization(login: $login) {
    projectsV2(first: $first, orderBy: {field: UPDATED_AT, direction: DESC}) {
      nodes {
        id
        number
        title
        url
        fields(first: 50) {
          nodes {
            __typename
            ... on ProjectV2FieldCommon {
              id
              name
              dataType
            }
            ... on ProjectV2SingleSelectField {
              id
              name
              dataType
              options { id name }
            }
          }
        }
      }
    }
  }
}
"""

PROJECT_ITEMS_QUERY = """
query ProjectItems($projectId: ID!, $first: Int!, $after: String) {
  node(id: $projectId) {
    ... on ProjectV2 {
      items(first: $first, after: $after) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id
          type
          content {
            __typename
            ... on Issue {
              id
              number
              title
              url
              state
              body
              repository { nameWithOwner }
              assignees(first: 10) { nodes { login } }
              labels(first: 10) { nodes { name } }
            }
            ... on PullRequest {
              id
              number
              title
              url
              state
              body
              repository { nameWithOwner }
              assignees(first: 10) { nodes { login } }
              labels(first: 10) { nodes { name } }
            }
            ... on DraftIssue {
              id
              title
              body
            }
          }
          fieldValues(first: 50) {
            nodes {
              __typename
              ... on ProjectV2ItemFieldTextValue {
                text
                field { ... on ProjectV2FieldCommon { id name dataType } }
              }
              ... on ProjectV2ItemFieldSingleSelectValue {
                name
                optionId
                field { ... on ProjectV2FieldCommon { id name dataType } }
              }
              ... on ProjectV2ItemFieldDateValue {
                date
                field { ... on ProjectV2FieldCommon { id name dataType } }
              }
              ... on ProjectV2ItemFieldNumberValue {
                number
                field { ... on ProjectV2FieldCommon { id name dataType } }
              }
              ... on ProjectV2ItemFieldUserValue {
                users(first: 10) { nodes { login } }
                field { ... on ProjectV2FieldCommon { id name dataType } }
              }
              ... on ProjectV2ItemFieldLabelValue {
                labels(first: 10) { nodes { name } }
                field { ... on ProjectV2FieldCommon { id name dataType } }
              }
              ... on ProjectV2ItemFieldMilestoneValue {
                milestone { title }
                field { ... on ProjectV2FieldCommon { id name dataType } }
              }
              ... on ProjectV2ItemFieldRepositoryValue {
                repository { nameWithOwner }
                field { ... on ProjectV2FieldCommon { id name dataType } }
              }
            }
          }
        }
      }
    }
  }
}
"""

REPOSITORY_ISSUE_QUERY = """
query RepositoryIssue($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    issue(number: $number) {
      id
      number
      title
      url
    }
  }
}
"""

ADD_ITEM_MUTATION = """
mutation AddItem($projectId: ID!, $contentId: ID!) {
  addProjectV2ItemById(input: {projectId: $projectId, contentId: $contentId}) {
    item { id }
  }
}
"""

ADD_DRAFT_MUTATION = """
mutation AddDraft($projectId: ID!, $title: String!, $body: String) {
  addProjectV2DraftIssue(input: {projectId: $projectId, title: $title, body: $body}) {
    projectItem { id }
  }
}
"""

DELETE_ITEM_MUTATION = """
mutation DeleteItem($projectId: ID!, $itemId: ID!) {
  deleteProjectV2Item(input: {projectId: $projectId, itemId: $itemId}) {
    deletedItemId
  }
}
"""

UPDATE_FIELD_MUTATION = """
mutation UpdateField($projectId: ID!, $itemId: ID!, $fieldId: ID!, $value: ProjectV2FieldValue!) {
  updateProjectV2ItemFieldValue(input: {
    projectId: $projectId,
    itemId: $itemId,
    fieldId: $fieldId,
    value: $value
  }) {
    projectV2Item { id }
  }
}
"""

UPDATE_DRAFT_MUTATION = """
mutation UpdateDraft($draftIssueId: ID!, $title: String, $body: String) {
  updateProjectV2DraftIssue(input: {
    draftIssueId: $draftIssueId,
    title: $title,
    body: $body
  }) {
    draftIssue { id title body }
  }
}
"""

UPDATE_ISSUE_MUTATION = """
mutation UpdateIssue($issueId: ID!, $title: String, $body: String) {
  updateIssue(input: {
    id: $issueId,
    title: $title,
    body: $body
  }) {
    issue { id number title url }
  }
}
"""


def token_from_env() -> str:
    token = os.environ.get("GH_TOKEN") or os.environ.get("GITHUB_TOKEN")
    if not token:
        raise GitHubError("Set GH_TOKEN or GITHUB_TOKEN with repo/project permissions.")
    return token


def list_projects(client: GitHubClient, owner: str) -> list[ProjectRef]:
    lookup_errors: list[str] = []
    for query, owner_key in ((USER_PROJECTS_QUERY, "user"), (ORG_PROJECTS_QUERY, "organization")):
        try:
            data = client.graphql(query, {"login": owner, "first": 50})
        except GitHubError as err:
            lookup_errors.append(str(err))
            continue

        owner_data = data.get(owner_key)
        if owner_data:
            return project_refs(owner_data["projectsV2"]["nodes"])

    if lookup_errors:
        raise GitHubError("; ".join(lookup_errors))
    raise GitHubError(f"Could not find GitHub user or organization {owner!r}.")


def project_refs(nodes: list[dict[str, Any]]) -> list[ProjectRef]:
    return [
        ProjectRef(
            id=node["id"],
            number=node["number"],
            title=node["title"],
            url=node["url"],
            fields=[field for field in node["fields"]["nodes"] if field],
        )
        for node in nodes
    ]


def resolve_project(args: argparse.Namespace, client: GitHubClient) -> ProjectRef:
    projects = list_projects(client, args.owner)
    if args.project_number is not None:
        for project in projects:
            if project.number == args.project_number:
                return project
        raise GitHubError(f"No project #{args.project_number} found for {args.owner}.")

    target_title = args.project_title.lower()
    for project in projects:
        if project.title.lower() == target_title:
            return project
    for project in projects:
        if target_title in project.title.lower():
            return project

    project_list = ", ".join(f"#{project.number} {project.title}" for project in projects)
    raise GitHubError(
        f"No project matching title {args.project_title!r}. "
        f"Pass --project-number. Available: {project_list or 'none'}"
    )


def status_field(project: ProjectRef) -> dict[str, Any]:
    for field in project.fields:
        if field.get("name", "").lower() == "status" and field.get("options"):
            return field
    raise GitHubError(f"Project {project.title!r} does not have a single-select Status field.")


def field_by_name(project: ProjectRef, name: str) -> dict[str, Any]:
    for field in project.fields:
        if field.get("name", "").lower() == name.lower():
            return field
    names = ", ".join(field.get("name", "") for field in project.fields)
    raise GitHubError(f"No project field named {name!r}. Available fields: {names}")


def option_by_name(field: dict[str, Any], name: str) -> dict[str, Any]:
    options = field.get("options") or []
    for option in options:
        if option["name"].lower() == name.lower():
            return option
    for option in options:
        if name.lower() in option["name"].lower():
            return option
    names = ", ".join(option["name"] for option in options)
    raise GitHubError(f"No option {name!r} for field {field['name']!r}. Available: {names}")


def list_items(client: GitHubClient, project_id: str, page_size: int = 100) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    after = None
    while True:
        data = client.graphql(
            PROJECT_ITEMS_QUERY,
            {"projectId": project_id, "first": page_size, "after": after},
        )
        node = data["node"]
        if not node:
            raise GitHubError("Project node was not found.")
        page = node["items"]
        items.extend(page["nodes"])
        if not page["pageInfo"]["hasNextPage"]:
            return items
        after = page["pageInfo"]["endCursor"]


def item_field_values(item: dict[str, Any]) -> dict[str, str]:
    values: dict[str, str] = {}
    for value in item["fieldValues"]["nodes"]:
        field = value.get("field") or {}
        name = field.get("name")
        if not name:
            continue
        typename = value["__typename"]
        if typename == "ProjectV2ItemFieldSingleSelectValue":
            values[name] = value.get("name") or ""
        elif typename == "ProjectV2ItemFieldTextValue":
            values[name] = value.get("text") or ""
        elif typename == "ProjectV2ItemFieldDateValue":
            values[name] = value.get("date") or ""
        elif typename == "ProjectV2ItemFieldNumberValue":
            values[name] = str(value.get("number") or "")
        elif typename == "ProjectV2ItemFieldUserValue":
            values[name] = ", ".join(user["login"] for user in value["users"]["nodes"])
        elif typename == "ProjectV2ItemFieldLabelValue":
            values[name] = ", ".join(label["name"] for label in value["labels"]["nodes"])
        elif typename == "ProjectV2ItemFieldMilestoneValue":
            milestone = value.get("milestone")
            values[name] = milestone["title"] if milestone else ""
        elif typename == "ProjectV2ItemFieldRepositoryValue":
            repo = value.get("repository")
            values[name] = repo["nameWithOwner"] if repo else ""
    return values


def item_title(item: dict[str, Any]) -> str:
    content = item.get("content") or {}
    return content.get("title") or "(untitled)"


def item_number(item: dict[str, Any]) -> str:
    content = item.get("content") or {}
    number = content.get("number")
    return f"#{number}" if number else "draft"


def item_url(item: dict[str, Any]) -> str:
    content = item.get("content") or {}
    return content.get("url") or ""


def item_content_id(item: dict[str, Any]) -> str:
    content = item.get("content") or {}
    return content.get("id") or ""


def markdown_escape(text: str) -> str:
    return text.replace("|", "\\|").replace("\n", " ")


def print_projects(projects: list[ProjectRef]) -> None:
    print("| Number | Title | URL |")
    print("| --- | --- | --- |")
    for project in projects:
        print(f"| {project.number} | {markdown_escape(project.title)} | {project.url} |")


def print_columns(project: ProjectRef) -> None:
    field = status_field(project)
    print(f"# {project.title} Status Columns")
    print()
    for option in field["options"]:
        print(f"- {option['name']} (`{option['id']}`)")


def print_items_markdown(items: list[dict[str, Any]], group: bool = True) -> None:
    if group:
        grouped: dict[str, list[dict[str, Any]]] = {}
        for item in items:
            status = item_field_values(item).get("Status", "No Status")
            grouped.setdefault(status, []).append(item)
        for status, status_items in grouped.items():
            print(f"## {status}")
            print()
            print("| Item | Title | Type | Assignees | Labels | URL |")
            print("| --- | --- | --- | --- | --- | --- |")
            for item in status_items:
                print(item_row(item))
            print()
        return

    print("| Status | Item | Title | Type | Assignees | Labels | URL |")
    print("| --- | --- | --- | --- | --- | --- | --- |")
    for item in items:
        status = item_field_values(item).get("Status", "No Status")
        print(f"| {markdown_escape(status)} | {item_row(item)}")


def item_row(item: dict[str, Any]) -> str:
    content = item.get("content") or {}
    assignees = ", ".join(user["login"] for user in content.get("assignees", {}).get("nodes", []))
    labels = ", ".join(label["name"] for label in content.get("labels", {}).get("nodes", []))
    return (
        f"| {item_number(item)} "
        f"| {markdown_escape(item_title(item))} "
        f"| {content.get('__typename', item.get('type', ''))} "
        f"| {markdown_escape(assignees)} "
        f"| {markdown_escape(labels)} "
        f"| {item_url(item)} |"
    )


def print_item_detail(item: dict[str, Any]) -> None:
    content = item.get("content") or {}
    values = item_field_values(item)
    print(f"# {item_number(item)} {item_title(item)}")
    print()
    print(f"- Project item ID: `{item['id']}`")
    if item_content_id(item):
        print(f"- Content ID: `{item_content_id(item)}`")
    print(f"- Type: `{content.get('__typename', item.get('type', ''))}`")
    if item_url(item):
        print(f"- URL: {item_url(item)}")
    for name, value in values.items():
        print(f"- {name}: {value}")
    body = content.get("body")
    if body:
        print()
        print("## Body")
        print()
        print(body)


def resolve_item(items: list[dict[str, Any]], ref: str) -> dict[str, Any]:
    clean = ref.strip()
    if clean.startswith("PVTI_"):
        for item in items:
            if item["id"] == clean:
                return item

    if clean.startswith("#"):
        clean = clean[1:]
    if clean.isdigit():
        for item in items:
            if str((item.get("content") or {}).get("number", "")) == clean:
                return item

    for item in items:
        content = item.get("content") or {}
        if clean in {item["id"], content.get("id", ""), content.get("url", "")}:
            return item
        if clean.lower() == item_title(item).lower():
            return item

    raise GitHubError(f"No project item matched {ref!r}. Use list to find the item ID.")


def issue_content_id(client: GitHubClient, owner: str, repo: str, number: int) -> str:
    data = client.graphql(REPOSITORY_ISSUE_QUERY, {"owner": owner, "repo": repo, "number": number})
    issue = data["repository"]["issue"] if data.get("repository") else None
    if not issue:
        raise GitHubError(f"No issue #{number} found in {owner}/{repo}.")
    return issue["id"]


def mutation_value(field: dict[str, Any], value: str) -> dict[str, Any]:
    data_type = field.get("dataType")
    if data_type == "SINGLE_SELECT":
        option = option_by_name(field, value)
        return {"singleSelectOptionId": option["id"]}
    if data_type == "DATE":
        return {"date": value}
    if data_type == "NUMBER":
        return {"number": float(value)}
    return {"text": value}


def cmd_projects(args: argparse.Namespace, client: GitHubClient) -> None:
    print_projects(list_projects(client, args.owner))


def cmd_columns(args: argparse.Namespace, client: GitHubClient) -> None:
    print_columns(resolve_project(args, client))


def cmd_list(args: argparse.Namespace, client: GitHubClient) -> None:
    project = resolve_project(args, client)
    items = list_items(client, project.id)
    if args.status:
        items = [
            item
            for item in items
            if item_field_values(item).get("Status", "").lower() == args.status.lower()
        ]
    print_items_markdown(items, group=not args.flat)


def cmd_show(args: argparse.Namespace, client: GitHubClient) -> None:
    project = resolve_project(args, client)
    item = resolve_item(list_items(client, project.id), args.item)
    print_item_detail(item)


def cmd_add_issue(args: argparse.Namespace, client: GitHubClient) -> None:
    project = resolve_project(args, client)
    content_id = issue_content_id(client, args.repo_owner, args.repo, args.number)
    data = client.graphql(ADD_ITEM_MUTATION, {"projectId": project.id, "contentId": content_id})
    item_id = data["addProjectV2ItemById"]["item"]["id"]
    print(f"Added issue #{args.number} to {project.title}: `{item_id}`")
    if args.status:
        set_status(client, project, item_id, args.status)
        print(f"Moved `{item_id}` to {args.status}.")


def cmd_add_draft(args: argparse.Namespace, client: GitHubClient) -> None:
    project = resolve_project(args, client)
    body = read_body_arg(args.body, args.body_file)
    data = client.graphql(ADD_DRAFT_MUTATION, {"projectId": project.id, "title": args.title, "body": body})
    item_id = data["addProjectV2DraftIssue"]["projectItem"]["id"]
    print(f"Added draft to {project.title}: `{item_id}`")
    if args.status:
        set_status(client, project, item_id, args.status)
        print(f"Moved `{item_id}` to {args.status}.")


def cmd_delete(args: argparse.Namespace, client: GitHubClient) -> None:
    project = resolve_project(args, client)
    item = resolve_item(list_items(client, project.id), args.item)
    client.graphql(DELETE_ITEM_MUTATION, {"projectId": project.id, "itemId": item["id"]})
    print(f"Deleted project item `{item['id']}`.")


def cmd_move(args: argparse.Namespace, client: GitHubClient) -> None:
    project = resolve_project(args, client)
    item = resolve_item(list_items(client, project.id), args.item)
    set_status(client, project, item["id"], args.status)
    print(f"Moved `{item['id']}` to {args.status}.")


def set_status(client: GitHubClient, project: ProjectRef, item_id: str, status: str) -> None:
    field = status_field(project)
    option = option_by_name(field, status)
    client.graphql(
        UPDATE_FIELD_MUTATION,
        {
            "projectId": project.id,
            "itemId": item_id,
            "fieldId": field["id"],
            "value": {"singleSelectOptionId": option["id"]},
        },
    )


def cmd_set_field(args: argparse.Namespace, client: GitHubClient) -> None:
    project = resolve_project(args, client)
    item = resolve_item(list_items(client, project.id), args.item)
    field = field_by_name(project, args.field)
    client.graphql(
        UPDATE_FIELD_MUTATION,
        {
            "projectId": project.id,
            "itemId": item["id"],
            "fieldId": field["id"],
            "value": mutation_value(field, args.value),
        },
    )
    print(f"Set {field['name']} on `{item['id']}` to {args.value!r}.")


def cmd_rename(args: argparse.Namespace, client: GitHubClient) -> None:
    project = resolve_project(args, client)
    item = resolve_item(list_items(client, project.id), args.item)
    content = item.get("content") or {}
    content_id = content.get("id")
    if not content_id:
        raise GitHubError("Cannot rename an item without content.")

    typename = content.get("__typename")
    body = read_body_arg(args.body, args.body_file)
    if body is None:
        body = content.get("body")
    variables = {"title": args.title, "body": body}
    if typename == "DraftIssue":
        variables["draftIssueId"] = content_id
        client.graphql(UPDATE_DRAFT_MUTATION, variables)
    elif typename == "Issue":
        variables["issueId"] = content_id
        client.graphql(UPDATE_ISSUE_MUTATION, variables)
    else:
        raise GitHubError(f"Rename is only supported for draft issues and issues, not {typename}.")
    print(f"Updated {typename} `{content_id}`.")


def read_body_arg(body: str | None, path: str | None) -> str | None:
    if path:
        with open(path, "r", encoding="utf-8") as handle:
            return handle.read()
    return body


def add_common_args(parser: argparse.ArgumentParser, suppress_defaults: bool = False) -> None:
    default = argparse.SUPPRESS if suppress_defaults else None
    parser.add_argument(
        "--owner",
        default=default if suppress_defaults else os.environ.get("CODEGYM_GITHUB_OWNER", DEFAULT_OWNER),
    )
    parser.add_argument(
        "--repo-owner",
        default=default if suppress_defaults else os.environ.get("CODEGYM_GITHUB_REPO_OWNER", DEFAULT_OWNER),
    )
    parser.add_argument(
        "--repo",
        default=default if suppress_defaults else os.environ.get("CODEGYM_GITHUB_REPO", DEFAULT_REPO),
    )
    parser.add_argument(
        "--project-number",
        type=int,
        default=default if suppress_defaults else env_int("CODEGYM_GITHUB_PROJECT_NUMBER"),
    )
    parser.add_argument(
        "--project-title",
        default=default if suppress_defaults else os.environ.get("CODEGYM_GITHUB_PROJECT_TITLE", DEFAULT_PROJECT_TITLE),
    )


def env_int(name: str) -> int | None:
    value = os.environ.get(name)
    return int(value) if value else None


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="View and manage the CodeGym GitHub Projects v2 kanban board.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=textwrap.dedent(
            """\
            Examples:
              python .agents/skills/codegym-project/scripts/github_project_board.py projects
              python .agents/skills/codegym-project/scripts/github_project_board.py columns --project-number 1
              python .agents/skills/codegym-project/scripts/github_project_board.py list --status Ready
              python .agents/skills/codegym-project/scripts/github_project_board.py add-issue 5 --status Ready
              python .agents/skills/codegym-project/scripts/github_project_board.py add-draft "Write CI workflow" --status Ready --body "..."
              python .agents/skills/codegym-project/scripts/github_project_board.py move 5 "In Progress"
              python .agents/skills/codegym-project/scripts/github_project_board.py set-field 5 Priority High
              python .agents/skills/codegym-project/scripts/github_project_board.py rename 5 "M1: CI workflow"
            """
        ),
    )
    add_common_args(parser)

    sub = parser.add_subparsers(dest="command", required=True)

    projects = sub.add_parser("projects", help="List owner projects.")
    add_common_args(projects, suppress_defaults=True)
    projects.set_defaults(func=cmd_projects)

    columns = sub.add_parser("columns", help="List project Status column options.")
    add_common_args(columns, suppress_defaults=True)
    columns.set_defaults(func=cmd_columns)

    list_parser = sub.add_parser("list", help="Render project items as Markdown.")
    add_common_args(list_parser, suppress_defaults=True)
    list_parser.add_argument("--status", help="Only show items in this Status column.")
    list_parser.add_argument("--flat", action="store_true", help="Do not group by Status.")
    list_parser.set_defaults(func=cmd_list)

    show = sub.add_parser("show", help="Expand one issue, draft, or PR as Markdown.")
    add_common_args(show, suppress_defaults=True)
    show.add_argument("item", help="Project item ID, issue number, URL, or exact title.")
    show.set_defaults(func=cmd_show)

    add_issue = sub.add_parser("add-issue", help="Add a repository issue to the project.")
    add_common_args(add_issue, suppress_defaults=True)
    add_issue.add_argument("number", type=int, help="Issue number in --repo.")
    add_issue.add_argument("--status", help="Optional Status column to move into after adding.")
    add_issue.set_defaults(func=cmd_add_issue)

    add_draft = sub.add_parser("add-draft", help="Add a draft issue to the project.")
    add_common_args(add_draft, suppress_defaults=True)
    add_draft.add_argument("title")
    add_draft.add_argument("--body")
    add_draft.add_argument("--body-file")
    add_draft.add_argument("--status", help="Optional Status column to move into after adding.")
    add_draft.set_defaults(func=cmd_add_draft)

    delete = sub.add_parser("delete", help="Delete an item from the project board.")
    add_common_args(delete, suppress_defaults=True)
    delete.add_argument("item", help="Project item ID, issue number, URL, or exact title.")
    delete.set_defaults(func=cmd_delete)

    move = sub.add_parser("move", help="Move an item to a Status column.")
    add_common_args(move, suppress_defaults=True)
    move.add_argument("item", help="Project item ID, issue number, URL, or exact title.")
    move.add_argument("status", help="Status column name, e.g. Ready.")
    move.set_defaults(func=cmd_move)

    set_field = sub.add_parser("set-field", help="Set a project field value on an item.")
    add_common_args(set_field, suppress_defaults=True)
    set_field.add_argument("item", help="Project item ID, issue number, URL, or exact title.")
    set_field.add_argument("field", help="Project field name.")
    set_field.add_argument("value", help="Field value. Single-select fields accept option names.")
    set_field.set_defaults(func=cmd_set_field)

    rename = sub.add_parser("rename", help="Rename an issue or draft issue, optionally replacing body.")
    add_common_args(rename, suppress_defaults=True)
    rename.add_argument("item", help="Project item ID, issue number, URL, or exact title.")
    rename.add_argument("title")
    rename.add_argument("--body")
    rename.add_argument("--body-file")
    rename.set_defaults(func=cmd_rename)

    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    try:
        client = GitHubClient(token_from_env())
        args.func(args, client)
        return 0
    except GitHubError as err:
        print(f"error: {err}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
