import { SparklesIcon, LoaderIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "react-hot-toast";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider } from "@/components/ui/tooltip";
import { memoServiceClient } from "@/grpcweb";
import { useTranslate } from "@/utils/i18n";
import { EditorRefActions } from "../Editor";

interface TagSuggestion {
  tag: string;
  reason: string;
}

interface Props {
  editorRef: React.RefObject<EditorRefActions>;
  contentLength: number;
  onRecommend: (tags: TagSuggestion[]) => void;
}

const TagRecommendButton = (props: Props) => {
  const t = useTranslate();
  const { editorRef, contentLength, onRecommend } = props;
  const [isLoading, setIsLoading] = useState(false);

  const handleRecommendClick = async () => {
    if (!editorRef.current || contentLength < 15 || isLoading) {
      return;
    }

    setIsLoading(true);

    // Create timeout promise
    const timeoutPromise = new Promise((_, reject) => {
      setTimeout(() => reject(new Error("timeout")), 15000); // 15 seconds timeout
    });

    try {
      const content = editorRef.current.getContent();

      // Extract existing tags from content (match backend regex pattern)
      const tagRegex = /#([a-zA-Z0-9_\-\u4e00-\u9fa5]+)/g;
      const existingTags = [...content.matchAll(tagRegex)].map((match) => match[1]);

      // Race between API call and timeout
      const response = await Promise.race([
        memoServiceClient.suggestMemoTags({
          content,
          existingTags,
        }),
        timeoutPromise,
      ]);

      const suggestedTags = (response as any).suggestedTags || [];

      // Convert the response format to our TagSuggestion interface
      const tagSuggestions: TagSuggestion[] = suggestedTags.map((item: any) => ({
        tag: item.tag || item, // Support both new format {tag, reason} and old format string
        reason: item.reason || "AI recommended",
      }));

      if (tagSuggestions.length === 0) {
        // No recommendations found
        toast(t("editor.tag-recommend.no-suggestions"), {
          icon: "💭",
          duration: 3000,
        });
      } else {
        // Has recommendations - pass to parent to show panel
        onRecommend(tagSuggestions);
      }
    } catch (error: any) {
      console.error("Failed to get tag recommendations:", error);
      if (error.message === "timeout") {
        // Timeout error
        toast.error(t("editor.tag-recommend.timeout"), {
          duration: 4000,
        });
      } else {
        // Other errors
        toast.error(t("editor.tag-recommend.error"));
      }
    } finally {
      setIsLoading(false);
    }
  };

  const isDisabled = contentLength < 15 || isLoading;

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            disabled={isDisabled}
            onClick={handleRecommendClick}
            className={isDisabled ? "opacity-40" : "opacity-60 hover:opacity-100"}
          >
            {isLoading ? <LoaderIcon className="size-5 animate-spin" /> : <SparklesIcon className="size-5" />}
          </Button>
        </TooltipTrigger>
        <TooltipContent side="bottom">
          <p>
            {isDisabled && contentLength < 15
              ? t("editor.tag-recommend.too-short")
              : isLoading
                ? t("editor.tag-recommend.loading")
                : t("editor.tag-recommend.tooltip")}
          </p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
};

export default TagRecommendButton;
