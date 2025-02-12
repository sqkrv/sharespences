from pydantic.alias_generators import to_camel

from backend.db import Base


class CamelBase(Base):
    class Config:
        alias_generator = to_camel
        populate_by_name = True
        from_attributes = True
