const groups = [
    { name: "Group name", date: "01.01.2000", spent: "1000 ₽", total: "1000 ₽" },
    { name: "Group name", date: "01.01.2000", spent: "1000 ₽", total: "1000 ₽" },
  ];
  
  export default function Groups() {
    return (
      <div className="w-full h-full  p-2 font-sans shadow-custom bg-background rounded-xl">
        <h2 className="text-h4 mb-2">Мои группы</h2>
        {groups.map((group, index) => (
          <div key={index} className="w-full h-auto bg-accent p-4 rounded-lg min-w-[100px] text-left flex flex-col justify-end pl-2 pb-2 leading-tight flex gap-1 mb-1">
            <p className="text-text">{group.name}</p>
            <p className="text-text">{group.date} {group.spent} / {group.total}</p>
          </div>
        ))}
      </div>
    );
  }