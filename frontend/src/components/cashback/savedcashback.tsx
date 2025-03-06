const subscriptions = [
    { name: "Sberbank", price: "1000 ₽", date: "Начислится 01.01.2000" },
    { name: "Tbank", price: "1000 ₽", date: "Начислится 01.01.2000" },
    { name: "AlfaBank", price: "1000 ₽", date: "Начислится 01.01.2000" },
  ];
  
  export default function Subscriptions() {
    return (
      <div className="w-full h-full  p-2 font-sans shadow-custom bg-background rounded-xl">
        <h2 className="text-h4 mb-2">Мои подписки</h2>
        {subscriptions.map((sub, index) => (
          <div key={index} className="w-full h-auto p-4 rounded-lg min-w-[100px] text-left flex flex-col justify-end pl-2 pb-2 leading-tight flex gap-1 mb-1">
            <p className="text-text">{sub.name}</p>
            <p className="text-text">{sub.price}</p>
            <p className="text-text">{sub.date}</p>
          </div>
        ))}
      </div>
    );
  }