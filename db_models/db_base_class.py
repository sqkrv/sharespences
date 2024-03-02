from sqlalchemy import JSON, Text
from sqlalchemy.orm import declarative_base


class Base:
    ...
    # id = Column(Integer, primary_key=True, index=True)
    # is_deleted = Column(Boolean, default=False, server_default="False")


Base = declarative_base(cls=Base, type_annotation_map={
    dict[str, str]: JSON,
    str: Text
})
