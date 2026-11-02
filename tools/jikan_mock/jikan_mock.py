import argparse
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Lock
from urllib.parse import parse_qs, urlsplit


def anime_records():
	records = []
	for mal_id in range(1, 52):
		record = {
			"mal_id": mal_id,
			"title": {1: "Mock Anime One", 2: "Mock Anime Two"}.get(
				mal_id, f"Mock Anime {mal_id:02}"
			),
			"title_english": f"Mock Anime English {mal_id:02}",
			"title_japanese": f"Mock Anime JP {mal_id:02}",
			"title_synonyms": [f"Anime Alias {mal_id:02}"],
			"type": "TV",
			"status": "Currently Airing" if mal_id % 5 == 2 else "Finished Airing",
			"airing": mal_id % 5 == 2,
			"episodes": {1: 12, 2: 24}.get(mal_id, 12 + mal_id),
			"score": {1: 8.0, 2: 7.5}.get(mal_id, 6.0 + (mal_id % 40) / 10),
			"synopsis": {
				1: "A completed mock anime.",
				2: "An ongoing mock anime.",
			}.get(mal_id, f"Mock anime {mal_id} synopsis."),
			"genres": [{"mal_id": 1, "name": "Action", "type": "anime"}],
		}
		if mal_id == 4:
			record["type"] = "Music"
		elif mal_id == 5:
			record["genres"] = [{"mal_id": 12, "name": "Hentai", "type": "anime"}]
		records.append(record)
	return records


def manga_records():
	records = []
	for mal_id in range(1, 52):
		record = {
			"mal_id": mal_id,
			"title": {1: "Mock Manga One", 2: "Mock Manga Two"}.get(
				mal_id, f"Mock Manga {mal_id:02}"
			),
			"title_english": f"Mock Manga English {mal_id:02}",
			"title_japanese": f"Mock Manga JP {mal_id:02}",
			"title_synonyms": [f"Manga Alias {mal_id:02}"],
			"type": "Manga",
			"status": "Publishing" if mal_id % 5 == 2 else "Finished",
			"chapters": {1: 100, 2: 20}.get(mal_id, 20 + mal_id),
			"volumes": {1: 10, 2: 2}.get(mal_id, 2 + mal_id % 12),
			"score": {1: 8.0, 2: 7.5}.get(mal_id, 6.0 + (mal_id % 40) / 10),
			"synopsis": {
				1: "A completed mock manga.",
				2: "An ongoing mock manga.",
			}.get(mal_id, f"Mock manga {mal_id} synopsis."),
			"genres": [{"mal_id": 1, "name": "Action", "type": "manga"}],
		}
		if mal_id == 4:
			record["type"] = "Novel"
		elif mal_id == 5:
			record["genres"] = [{"mal_id": 12, "name": "Hentai", "type": "manga"}]
		records.append(record)
	return records


RECORDS = {
	"anime": anime_records(),
	"manga": manga_records(),
}


class JikanHandler(BaseHTTPRequestHandler):
	_failure_lock = Lock()
	_failures = {}

	@classmethod
	def set_failures(cls, resource, statuses):
		with cls._failure_lock:
			cls._failures[tuple(resource)] = list(statuses)

	@classmethod
	def clear_failures(cls, resource):
		with cls._failure_lock:
			cls._failures.pop(tuple(resource), None)

	@classmethod
	def _take_failure(cls, resource):
		with cls._failure_lock:
			statuses = cls._failures.get(tuple(resource))
			if not statuses:
				return None
			status = statuses.pop(0)
			if not statuses:
				del cls._failures[tuple(resource)]
			return status

	def do_GET(self):
		request = urlsplit(self.path)
		parts = request.path.strip("/").split("/")
		if parts[0] in {"v1", "v4"}:
			parts = parts[1:]
		failure_status = self._take_failure(parts)
		if failure_status is not None:
			self.respond(failure_status, {
				"status": failure_status,
				"message": "Configured mock failure",
			})
			return
		if not parts or parts[0] not in RECORDS or len(parts) > 2:
			self.respond(404, {"status": 404, "message": "Not found"})
			return

		records = RECORDS[parts[0]]
		if len(parts) == 2:
			record = next(
				(record for record in records if str(record["mal_id"]) == parts[1]),
				None,
			)
			if record is None:
				self.respond(404, {"status": 404, "message": "Not found"})
			else:
				self.respond(200, {"data": record})
			return

		query = parse_qs(request.query)
		try:
			page = int(query.get("page", ["1"])[0])
			limit = int(query.get("limit", ["25"])[0])
			if page < 1 or limit < 1:
				raise ValueError
		except ValueError:
			self.respond(400, {"status": 400, "message": "Invalid page or limit"})
			return

		start = (page - 1) * limit
		data = records[start:start + limit]
		last_page = max(1, (len(records) + limit - 1) // limit)
		self.respond(200, {
			"data": data,
			"pagination": {
				"last_visible_page": last_page,
				"has_next_page": page < last_page,
				"current_page": page,
				"items": {"count": len(data), "total": len(records), "per_page": limit},
			},
		})

	def respond(self, status, payload):
		body = json.dumps(payload).encode("utf-8")
		self.send_response(status)
		self.send_header("Content-Type", "application/json")
		self.send_header("Content-Length", str(len(body)))
		self.end_headers()
		self.wfile.write(body)


if __name__ == "__main__":
	parser = argparse.ArgumentParser()
	parser.add_argument("--port", type=int, default=9999)
	args = parser.parse_args()
	with ThreadingHTTPServer(("127.0.0.1", args.port), JikanHandler) as server:
		print(f"Jikan mock: http://127.0.0.1:{server.server_port}/v4", flush=True)
		try:
			server.serve_forever()
		except KeyboardInterrupt:
			pass
