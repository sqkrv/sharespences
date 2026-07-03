const group = [
    { name: "Название группы", price: "1000 ₽", date: "01.01.2000" },
    { name: "Название группы", price: "1000 ₽", date: "01.01.2000" },
    { name: "Название группы", price: "1000 ₽", date: "01.01.2000" },
    { name: "Название группы", price: "1000 ₽", date: "01.01.2000" },
  ];
  
  export default function Subscriptions() {
    return (
      <div className="w-full h-full  p-2 font-sans shadow-custom bg-background rounded-xl">
        <h2 className="text-h4 mb-2">Мои группы</h2>
        {group.map((group, index) => (
          <div key={index} className="w-full h-auto p-4 rounded-lg min-w-[100px] text-left flex flex-col justify-end pl-2 pb-2 leading-tight flex gap-1 mb-1 bg-accent">
            <p className="text-h4">{group.name}</p>
            <p className="text-text">{group.price}</p>
            <p className="text-text text-shade">{group.date}</p>
          </div>
        ))}
      </div>
    );
  }