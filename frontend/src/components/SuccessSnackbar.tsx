import React, { forwardRef } from "react";
import { SnackbarContent, CustomContentProps } from "notistack";

interface CustomSnackbarProps extends CustomContentProps {
  type: "success" | "info" | "error";
}

const SuccessSnackbar = forwardRef<HTMLDivElement, CustomSnackbarProps>(
  (props, ref) => {
    const { id, message, type, ...other } = props;

    return (
      <SnackbarContent
        ref={ref}
        role="alert"
        {...other}
        style={{
          marginRight: 60,
          position: "inherit",
        }}
        className="rounded-r-[10px] py-[10px] px-5 !bg-primary !bottom-0"
      >
        <div className="flex gap-4 items-center">
          <p className="text-base lg:text-xl text-white font-normal">{message}</p>
        </div>
      </SnackbarContent>
    );
  }
);

SuccessSnackbar.displayName = "SuccessSnackbar";

export default SuccessSnackbar;
