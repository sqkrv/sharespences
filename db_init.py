from sqlalchemy import insert

from db_models.db_base_class import Base
import db_models.models
from db_models.models.other_models import BankDB
from db_models.db_session import SessionLocal, sync_engine

if __name__ == '__main__':
    with SessionLocal() as session:

        print(Base.metadata.tables.keys())
        Base.metadata.drop_all(bind=sync_engine)
        Base.metadata.create_all(bind=sync_engine)
        print(Base.metadata.tables.keys())

        banks = [
                {"name": "Альфа-Банк"},
                {"name": "Сбербанк"},
                {"name": "Тинькофф"},
        ]
        session.execute(insert(BankDB), banks)

        session.commit()
