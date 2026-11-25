// 文件说明：cronjob_target.go — 定时任务「目标」的校验与群绑定：云函数目标是否存在生效版、助手 / 群选项是否合法、群绑定行的读写
//
// 用法
//
//	同包调用：validateTargetFor() / saveGroupBinding() ← cronjob_handler.go 的 Create / Update；
//	isTargetMissing() / buildAgentIndex() / buildTargetIndex() ← List（target_missing 标记）
//
// 调用架构
//
//	[上游] internal/app
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/api/cronjob_target.go
//	         │ 依赖
//	         ▼
//	[下游] internal/systemdb
//	[同包] cronjob_handler.go（handler、请求体校验）
//
// 主要符号
//
//	(*CronJobHandler).validateTarget / validateTargetFor / isTargetMissing
//	(*CronJobHandler).lookupGroup / saveGroupBinding
//	(*CronJobHandler).buildAgentIndex / buildTargetIndex

package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/linkxzhou/SimpleBase/internal/agentrun"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// validateTarget 校验目标云函数存在生效版且函数在生效版 exports 内（plan §2.1）。
func (h *CronJobHandler) validateTarget(ctx context.Context, projectID, file, export string) error {
	v, err := h.store.ResolveActiveSource(ctx, projectID, file)
	if errors.Is(err, sql.ErrNoRows) {
		return NewAPIError(http.StatusBadRequest, "gofunction_not_found", "target go function file does not exist", RequestIDFromContext(ctx))
	}
	if errors.Is(err, systemdb.ErrNoActiveVersion) {
		return NewAPIError(http.StatusBadRequest, "no_active_version", "go function has no active version", RequestIDFromContext(ctx))
	}
	if err != nil {
		return err
	}
	for _, e := range v.Exports {
		if e == export {
			return nil
		}
	}
	return NewAPIError(http.StatusBadRequest, "function_not_exported", "target function is not exported in this file", RequestIDFromContext(ctx))
}

// validateTargetFor 按 target_kind 分支校验目标：function 走生效版校验；
// agent 只校验云助手存在（不复用 gofunction 校验）。
func (h *CronJobHandler) validateTargetFor(ctx context.Context, projectID string, req upsertCronJobBody) error {
	if req.TargetKind != systemdb.CronJobTargetAgent {
		return h.validateTarget(ctx, projectID, req.FuncFile, req.FuncExport)
	}
	_, err := h.store.GetCloudAgent(ctx, projectID, req.AgentID)
	if errors.Is(err, sql.ErrNoRows) {
		return NewAPIError(http.StatusBadRequest, "agent_not_found", "agent does not exist in this project", RequestIDFromContext(ctx))
	}
	if err != nil {
		return err
	}
	// v2：绑定了群时，群必须存在且包含该助手（否则调度时一定失败）。
	if req.TagID == "" {
		return nil
	}
	tag, err := h.lookupGroup(ctx, projectID, req.TagID)
	if err != nil {
		return err
	}
	for _, id := range tag.Config.AgentIDs {
		if id == req.AgentID {
			return nil
		}
	}
	return NewAPIError(http.StatusBadRequest, "group_member_unavailable",
		"agent_id must be a member of the bound group", RequestIDFromContext(ctx))
}

// lookupGroup 解析群（内置群缺省行也可见）；未找到 → 400。
func (h *CronJobHandler) lookupGroup(ctx context.Context, projectID, tagID string) (systemdb.AgentTag, error) {
	tag, err := h.store.GetAgentTagRow(ctx, projectID, tagID)
	if errors.Is(err, sql.ErrNoRows) {
		if !systemdb.IsBuiltinAgentTag(tagID) {
			return systemdb.AgentTag{}, NewAPIError(http.StatusBadRequest, "agent_tag_not_found",
				"agent tag not found", RequestIDFromContext(ctx))
		}
		if tagID == systemdb.AgentTagGeneral {
			agents, err := h.store.ListCloudAgents(ctx, projectID)
			if err != nil {
				return systemdb.AgentTag{}, err
			}
			return agentrun.GeneralAgentTag(projectID, agents), nil
		}
		return systemdb.AgentTag{ID: tagID, ProjectID: projectID, BuiltinKey: tagID}, nil
	}
	if err != nil {
		return systemdb.AgentTag{}, err
	}
	return tag, nil
}

// saveGroupBinding 写入/更新群定时绑定；req.TagID 为空时归档既有绑定，
// 让任务回到「单助手」语义（用户显式解除群绑定）。
func (h *CronJobHandler) saveGroupBinding(ctx context.Context, projectID, jobID string, req upsertCronJobBody) error {
	if req.TargetKind != systemdb.CronJobTargetAgent {
		return nil
	}
	if req.TagID == "" {
		return h.store.ArchiveAgentGroupCron(ctx, projectID, jobID)
	}
	policy := req.ThreadPolicy
	if policy == "" {
		policy = systemdb.GroupCronReuseThread
	}
	kind := req.ThreadKind
	if kind == "" {
		kind = systemdb.ThreadKindScheduled
	}
	// 保留既有 thread_id：改提示词/换负责人不应把会话重建。
	existing, err := h.store.GetAgentGroupCron(ctx, projectID, jobID)
	if err != nil {
		return err
	}
	threadID := existing.ThreadID
	if existing.TagID != req.TagID {
		// 换了群：旧会话不再适用。
		threadID = ""
	}
	_, err = h.store.UpsertAgentGroupCron(ctx, systemdb.AgentGroupCron{
		ProjectID: projectID, JobID: jobID, TagID: req.TagID,
		ThreadPolicy: policy, ThreadKind: kind, ThreadID: threadID,
		OwnerUserID: req.OwnerUserID, Prompt: req.AgentPrompt,
	})
	return err
}

// buildAgentIndex 构造项目内存在的云助手 ID 集合，用于 agent 任务的 target_missing 标记。
func (h *CronJobHandler) buildAgentIndex(ctx context.Context, projectID string) (map[string]bool, error) {
	agents, err := h.store.ListCloudAgents(ctx, projectID)
	if err != nil {
		return nil, err
	}
	index := make(map[string]bool, len(agents))
	for _, a := range agents {
		index[a.ID] = true
	}
	return index, nil
}

// buildTargetIndex 构造项目内可用目标集合（仅生效版 exports）：file\x00export → true。
func (h *CronJobHandler) buildTargetIndex(ctx context.Context, projectID string) (map[string]bool, error) {
	funcs, err := h.store.ListGoFuncs(ctx, projectID)
	if err != nil {
		return nil, err
	}
	index := map[string]bool{}
	for _, f := range funcs {
		if f.ActiveVersion <= 0 {
			continue
		}
		v, err := h.store.GetGoFuncVersion(ctx, projectID, f.Name, f.ActiveVersion)
		if err != nil {
			continue
		}
		for _, e := range v.Exports {
			index[f.Name+"\x00"+e] = true
		}
	}
	return index, nil
}

// isTargetMissing 单个目标可用性检查。
func (h *CronJobHandler) isTargetMissing(ctx context.Context, projectID, file, export string) (bool, error) {
	index, err := h.buildTargetIndex(ctx, projectID)
	if err != nil {
		return false, err
	}
	return !index[file+"\x00"+export], nil
}
