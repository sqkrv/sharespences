from pydantic.alias_generators import to_camel, to_snake
from sqlmodel import Numeric
from sqlmodel import SQLModel as _SQLModel
from sqlmodel.main import declared_attr


class Base(_SQLModel):
    @declared_attr
    def __tablename__(cls) -> str:
        return to_snake(cls.__name__)


class CamelBase(Base):
    class Config:
        alias_generator = to_camel
        populate_by_name = True
        from_attributes = True


money_type = Numeric(19, 4)
