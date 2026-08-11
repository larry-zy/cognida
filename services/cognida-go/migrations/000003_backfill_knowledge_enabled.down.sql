-- 不可逆数据回填：原始每行的 disabled/enabled 状态在正向回填后已不可区分，
-- 无法安全还原（无法判断某行历史上究竟是「误默认停用」还是「用户显式停用」）。
-- 故 down 为有意的 no-op，仅占位以满足 golang-migrate 成对文件约定。
SELECT 1;
