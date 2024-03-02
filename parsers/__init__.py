from abc import ABC, abstractmethod


class Parser(ABC):
    @abstractmethod
    def __init__(self):
        ...

    @abstractmethod
    async def parse_operations(self):
        ...
