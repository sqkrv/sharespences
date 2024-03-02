# class Credential

class PasskeyRepository:
    def __init__(self):
        self.credentials = {}

    def add_credential(self, username: str, credential: str):
        self.credentials[username] = credential

    def get_credential(self, username: str) -> str | None:
        return self.credentials.get(username, None)
