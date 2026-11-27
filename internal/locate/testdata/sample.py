import os


@decorator
def top(a, b):
    """Docstring with def fake(): inside."""
    def inner():
        return 1
    return inner()


class Repo(Base):
    limit = 3

    def __init__(self, path):
        self.path = path

    @property
    def size(self):
        # def comment():
        return len(self.path)

    async def fetch(
        self,
        url,
    ):
        return url


def last():
    pass
