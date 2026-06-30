# Web 前端规范检查

## 1. 命名规范

### 组件名
- ✅ 驼峰命名：`UserProfile`, `DataTable`
- ❌ 缩写：`UserProfileCmp`

### 文件名
- React: 驼峰 `UserProfile.tsx` 或小写+短横线 `user-profile.tsx`
- Vue: 短横线 `user-profile.vue`

### CSS 类名
- ✅ 短横线：`.user-profile`, `.btn-primary`
- ❌ 驼峰：`.userProfile`
- ❌ 下划线：`.user_profile`

### 常量
- ✅ 大写+下划线：`MAX_ITEMS`, `API_ENDPOINT`
- ❌ 小写：`maxItems`

## 2. 组件设计

### 单一职责
```tsx
// ✅ 正确 - 组件只负责一件事
function UserProfile({ userId }: { userId: string }) {
  const user = useUser(userId);
  return <div>{user.name}</div>;
}

// ❌ 错误 - 组件职责过多
function UserProfile({ userId }: { userId: string }) {
  const user = useUser(userId);
  const posts = useUserPosts(userId);
  const analytics = useAnalytics(userId);
  return <div>{/* 太多逻辑 */}</div>;
}
```

### Props 类型定义
```tsx
// ✅ 正确
interface ButtonProps {
  label: string;
  onClick: () => void;
  disabled?: boolean;
}

// ❌ 错误 - 缺少类型
function Button({ label, onClick, disabled }) {
  return <button>{label}</button>;
}
```

## 3. 状态管理

### Hooks 规则
```tsx
// ✅ 正确 - 只在顶层调用 Hooks
function UserProfile() {
  const [user, setUser] = useState(null);
  const posts = usePosts(user?.id);
  return <div>{/* ... */}</div>;
}

// ❌ 错误 - 在条件中调用 Hooks
function UserProfile() {
  if (condition) {
    const [user, setUser] = useState(null); // 错误!
  }
  return <div />;
}
```

### 状态提升
```tsx
// ✅ 正确 - 状态在父组件管理
function Parent() {
  const [active, setActive] = useState(false);
  return (
    <>
      <Child isActive={active} onToggle={() => setActive(!active)} />
    </>
  );
}

// ❌ 可避免 - 子组件重复状态
function Child({ isActive }: { isActive: boolean }) {
  const [localActive, setLocalActive] = useState(isActive);
  // 不必要的重复状态
}
```

## 4. 性能优化

### 避免不必要的渲染
```tsx
// ✅ 正确 - 使用 memo
const ExpensiveComponent = memo(({ data }: Props) => {
  return <div>{/* 渲染逻辑 */}</div>;
});

// ✅ 正确 - 使用 useMemo
function DataList({ items }: { items: Item[] }) {
  const sorted = useMemo(() => items.sort(byDate), [items]);
  return <ul>{sorted.map(item => <li key={item.id}>{item.name}</li>)}</ul>;
}
```

### 避免内联函数
```tsx
// ✅ 正确 - 使用 useCallback
function Parent() {
  const handleClick = useCallback(() => {
    console.log('clicked');
  }, []);
  return <Child onClick={handleClick} />;
}

// ❌ 可优化
function Parent() {
  return <Child onClick={() => console.log('clicked')} />;
}
```

## 5. 错误处理

### 错误边界
```tsx
// ✅ 正确 - 使用 Error Boundary
class ErrorBoundary extends React.Component {
  state = { hasError: false };

  static getDerivedStateFromError(error: Error) {
    return { hasError: true };
  }

  render() {
    if (this.state.hasError) {
      return <ErrorFallback />;
    }
    return this.props.children;
  }
}
```

### 异步错误处理
```tsx
// ✅ 正确 - 处理异步错误
function UserProfile({ userId }: { userId: string }) {
  const [error, setError] = useState<Error | null>(null);
  const [user, setUser] = useState<User | null>(null);

  useEffect(() => {
    fetchUser(userId)
      .then(setUser)
      .catch(setError);
  }, [userId]);

  if (error) return <ErrorMessage error={error} />;
  if (!user) return <Loading />;
  return <div>{user.name}</div>;
}
```

## 6. 类型安全

### 避免 any
```tsx
// ✅ 正确 - 使用具体类型
interface User {
  id: string;
  name: string;
}

function UserCard({ user }: { user: User }) {
  return <div>{user.name}</div>;
}

// ❌ 错误 - 使用 any
function UserCard({ user }: { user: any }) {
  return <div>{user.name}</div>;
}
```

### 使用非空断言谨慎
```tsx
// ✅ 谨慎使用 - 确保值非空
const user = data.user!;
// 最好先检查
const user = data.user ?? DEFAULT_USER;
```

## 7. 样式规范

### CSS-in-JS
```tsx
// ✅ 推荐 - CSS Modules
import styles from './UserProfile.module.css';

function UserProfile() {
  return <div className={styles.container} />;
}

// ✅ 推荐 - Styled Components
const Container = styled.div`
  padding: 20px;
  background: white;
`;

// ❌ 避免 - 内联样式（除非动态）
function UserProfile() {
  return <div style={{ padding: '20px', background: 'white' }} />;
}
```

### 响应式设计
```css
/* ✅ 正确 - 移动优先 */
.container {
  padding: 10px;
}

@media (min-width: 768px) {
  .container {
    padding: 20px;
  }
}
```

## 8. 可访问性

### ARIA 属性
```tsx
// ✅ 正确 - 添加可访问性属性
<button
  aria-label="关闭对话框"
  onClick={onClose}
>
  ×
</button>

// ✅ 正确 - 语义化 HTML
<nav aria-label="主导航">
  <a href="/">首页</a>
</nav>
```

### 键盘导航
```tsx
// ✅ 正确 - 支持键盘操作
<div
  tabIndex={0}
  onKeyDown={(e) => {
    if (e.key === 'Enter') onClick();
  }}
  onClick={onClick}
>
  可点击元素
</div>
```

## 9. API 调用

### 统一错误处理
```tsx
// ✅ 正确 - 统一处理
const api = {
  async getUser(id: string): Promise<User> {
    try {
      const response = await fetch(`/api/users/${id}`);
      if (!response.ok) throw new ApiError(response.status);
      return response.json();
    } catch (error) {
      handleError(error);
      throw error;
    }
  }
};
```

### 请求取消
```tsx
// ✅ 正确 - 取消未完成请求
function UserProfile({ userId }: { userId: string }) {
  useEffect(() => {
    const controller = new AbortController();
    fetchUser(userId, controller.signal)
      .then(setUser)
      .catch(setError);

    return () => controller.abort();
  }, [userId]);
}
```

## 10. 测试

### 组件测试
```tsx
// ✅ 正确 - 测试组件行为
test('用户资料显示用户名', () => {
  render(<UserProfile userId="123" />);
  expect(screen.getByText('John Doe')).toBeInTheDocument();
});
```

### 用户交互测试
```tsx
// ✅ 正确 - 测试用户交互
test('点击按钮切换状态', () => {
  render(<ToggleButton />);
  fireEvent.click(screen.getByRole('button'));
  expect(screen.getByRole('button')).toHaveAttribute('aria-pressed', 'true');
});
```

## 11. 常见错误模式

### useEffect 依赖
```tsx
// ❌ 错误 - 缺少依赖
useEffect(() => {
  fetchData(userId);
  // eslint-disable-line react-hooks/exhaustive-deps
}, []);

// ✅ 正确 - 包含所有依赖
useEffect(() => {
  fetchData(userId);
}, [userId]);
```

### 状态更新闭包陷阱
```tsx
// ❌ 错误 - 闭包陷阱
function Counter() {
  const [count, setCount] = useState(0);
  useEffect(() => {
    const timer = setInterval(() => {
      setCount(count + 1); // count 永远是 0
    }, 1000);
    return () => clearInterval(timer);
  }, []);

  // ✅ 正确 - 使用函数式更新
  useEffect(() => {
    const timer = setInterval(() => {
      setCount(c => c + 1);
    }, 1000);
    return () => clearInterval(timer);
  }, []);
}
```

## 12. 代码组织

### 文件结构
```
components/
├── user/
│   ├── UserProfile.tsx
│   ├── UserProfile.test.tsx
│   ├── UserAvatar.tsx
│   └── index.ts
```

### 导出顺序
```tsx
// 1. 类型导入
import type { User } from './types';

// 2. 值导入
import { useUser } from './hooks';

// 3. 本地类型
interface Props { }

// 4. 组件定义
export function UserProfile() { }
```

## 13. 安全注意

### XSS 防护
```tsx
// ✅ React 默认转义
<div>{userInput}</div>

// ❌ 危险 - 直接渲染 HTML
<div dangerouslySetInnerHTML={{ __html: userInput }} />

// ✅ 如需渲染 HTML，先清理
import DOMPurify from 'dompurify';
<div dangerouslySetInnerHTML={{ __html: DOMPurify.sanitize(userInput) }} />
```

### CSRF 防护
```tsx
// ✅ 正确 - 发送 CSRF token
fetch('/api/users', {
  method: 'POST',
  headers: {
    'X-CSRF-Token': csrfToken,
  },
});
```

## 14. 编译前检查

```bash
# 类型检查
npm run type-check

# Lint
npm run lint

# 测试
npm run test

# 构建
npm run build
```
