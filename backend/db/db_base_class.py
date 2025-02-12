from pydantic.alias_generators import to_snake
from sqlmodel import SQLModel as _SQLModel
from sqlmodel.main import declared_attr


class Base(_SQLModel):
    @declared_attr
    def __tablename__(cls) -> str:
        return to_snake(cls.__name__)
