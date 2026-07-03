const cashbacks = [
  { percent: "5%", category: "Продукты" },
  { percent: "5%", category: "Продукты" },
  { percent: "5%", category: "Продукты" },
  { percent: "5%", category: "Продукты" },
  { percent: "5%", category: "Продукты" },
  { percent: "5%", category: "Продукты" },
];

export default function Cashback() {
  return (
      <div className="w-full h-[220px] p-2 font-sans shadow-custom bg-background rounded-xl">
          <h2 className="text-h4 mb-2">Актуальные категории</h2>
          <div className="grid grid-cols-3 gap-1">
              {cashbacks.map((cashback, index) => (
                  <div 
                      key={index} 
                      className="bg-accent p-4 rounded-lg text-left flex flex-col justify-end pl-2 pb-2 leading-tight"
                  >
                      <p className="text-h2">{cashback.percent}</p>
                      <p className="text-text">{cashback.category}</p>
                  </div>
              ))}
          </div>
      </div>
  );
}