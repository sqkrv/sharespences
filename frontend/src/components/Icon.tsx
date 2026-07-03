import React from "react";

type IconNames = 
  | "groups" 
  | "add_circle" 
  | "add"
  | "back"
  | "forward"
  | "cards"
  | "cb-ful"
  | "cb"
  | "history"
  | "home"
  | "refresh"
  | "savings"
  | "search"
  | "settings"

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
