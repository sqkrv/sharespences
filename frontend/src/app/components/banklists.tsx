const banks = [
    { name: "Сбербанк", amount: "100 000", currency: "₽", icon: "sberbank-of-russia.png" },
    { name: "Тинькофф", amount: "50 000", currency: "₽", icon: "tinkoff-bank.png" },
    { name: "Альфа-банк", amount: "30 000", currency: "₽", icon: "alfabank.png" },
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
        <div className="flex flex-col font-sans w-full h-[132px] bg-background">
          {banks.map((bank, index) => (
            <div
              key={index}
              className="relative bg-background p-2 rounded-lg w-full h-[37px] shadow-md flex flex-col justify-between"
            >
              <div className="flex justify-between">
                <div className="flex items-center gap-2">
                  <img src={bank.icon} alt={bank.name} className="w-6 h-6" />
                  <p className="text-text text-sm">{bank.name}</p>
                </div>
                <div className="flex items-baseline gap-1">
                  <p className="text-text text-sm">{bank.amount}</p>
                  <p className="text-text text-sm">{bank.currency}</p>
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>
    );
  }
  