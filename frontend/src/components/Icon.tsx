import React from "react";

type IconNames = 
  | "add"
  | "icon"
  | "search"
  | "history"

type Props = {
  name: IconNames;
  className?: string;
};

const Icon: React.FC<Props> = ({ name, className }) => {
  return (
    <svg className={`transition-all duration-300 ${className}`}>
      <use
        className="w-full h-full object-contain"
        href={`/icons/sprite.svg#${name}`}
      ></use>
    </svg>
  );
};

export default Icon;
