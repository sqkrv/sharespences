export default function Buttons() {
    return (
      <div className="w-full h-[132px] bg-background rounded-xl p-2 flex gap-1 font-sans text-text shadow-custom">
        <button className="w-full h-full bg-accent rounded-lg flex flex-col justify-end pl-2 pb-4 text-left text-font text-text leading-tight whitespace-pre-line">
          <span>Перевести</span>
          <span>деньги</span>
        </button>
        <button className="w-full h-full bg-accent rounded-lg flex flex-col justify-end pl-2 pb-4 text-left text-font text-text leading-tight whitespace-pre-line">
          <span>Новая</span>
          <span>группа</span>
        </button>
        <button className="w-full h-full bg-accent rounded-lg flex flex-col justify-end pl-2 pb-4 text-left text-font text-text leading-tight whitespace-pre-line">
          <span>Выбрать</span>
          <span>кешбек</span>
        </button>
      </div>
    );
  }
  