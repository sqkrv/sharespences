const banks = [
    { name: "Сбербанк", amount: "100 000", currency: "₽", icon: "sberbank-of-russia.png", color: "#FFE2E2" },
    { name: "Тинькофф", amount: "50 000", currency: "₽", icon: "tinkoff-bank.png", color: "#FFFED6" },
    { name: "Альфа-банк", amount: "30 000", currency: "₽", icon: "alfabank.png", color: "#D8ECD7" },
  ];
  
  export default function Banklist() {
    return (
      <div className="flex flex-col gap-6 px-2 font-sans">
        {/* Баланс */}
        <div className="flex flex-col gap-1">
          <p className="text-h4 text-font">Ваш баланс</p>
          <div className="flex items-baseline gap-1">
            <p className="text-h2 text-font">100 000</p>
            <p className="text-h2 text-font">₽</p>
          </div>
        </div>
        
        {/* Карточки банков */}
        <div className="flex flex-col font-sans w-full bg-background">
          {banks.map((bank, index) => (
            <div
              key={index}
              className={`relative p-2 rounded-lg w-full h-[180px] shadow-md flex flex-col justify-between ${index !== 0 ? "-mt-36" : ""}`}
              style={{background: bank.color}}
            >
              <div className="flex justify-between">
                <div className="flex items-center gap-2">
                  <img src={bank.icon} alt={bank.name} className="w-6 h-6" />
                  <p className="text-text text-sm text-black">{bank.name}</p>
                </div>
                <div className="flex items-baseline gap-1">
                  <p className="text-text text-sm text-black">{bank.amount}</p>
                  <p className="text-text text-sm text-black">{bank.currency}</p>
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>
    );
  }
  