export default function ButtonsGroups() {
    return (
      <div className="w-full h-[96px] bg-background rounded-xl p-2 flex gap-1 font-sans text-text shadow-custom">
        <button className="w-full h-full bg-accent rounded-lg flex flex-col justify-end pl-2 pb-2 text-left text-font text-h4 leading-tight whitespace-pre-line">
          <span>Новая</span>
          <span>группа</span>
        </button>
        <button className="w-full h-full bg-accent rounded-lg flex flex-col justify-end pl-2 pb-2 text-left text-font text-h4 leading-tight whitespace-pre-line">
          <span>История</span>
          <span>групп</span>
        </button>
      </div>
    );
  }
  