from sqlalchemy.orm import Mapped, mapped_column

from backend.models import Base


class ArticleDB(Base):
    __tablename__ = "news"

    id: Mapped[int] = mapped_column(primary_key=True)
    title: Mapped[str]
    text: Mapped[str]
