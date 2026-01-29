local HttpService = game:GetService("HttpService")

local Serializer = {}

local function GenerateGUID()
	return HttpService:GenerateGUID(false)
end

local function SerializeVector2(v)
	return {
		X = v.X,
		Y = v.Y,
	}
end

local function SerializeUDim(u)
	return {
		Scale = u.Scale,
		Offset = u.Offset,
	}
end

local function SerializeUDim2(u)
	return {
		X = SerializeUDim(u.X),
		Y = SerializeUDim(u.Y),
	}
end

local function SerializeColor3(c)
	return {
		R = c.R,
		G = c.G,
		B = c.B,
	}
end

local function SerializeNumberSequence(ns)
	local keypoints = {}
	for _, kp in ipairs(ns.Keypoints) do
		table.insert(keypoints, {
			Time = kp.Time,
			Value = kp.Value,
			Envelope = kp.Envelope,
		})
	end
	return {
		Keypoints = keypoints,
	}
end

local function SerializeColorSequence(cs)
	local keypoints = {}
	for _, kp in ipairs(cs.Keypoints) do
		table.insert(keypoints, {
			Time = kp.Time,
			Value = SerializeColor3(kp.Value),
		})
	end
	return {
		Keypoints = keypoints,
	}
end

local function SerializeUIGradient(gradient)
	if not gradient then
		return nil
	end

	return {
		Enabled = gradient.Enabled,
		Rotation = gradient.Rotation,
		Transparency = SerializeNumberSequence(gradient.Transparency),
		Color = SerializeColorSequence(gradient.Color),
		Offset = SerializeVector2(gradient.Offset),
	}
end

local function FindChildOfClass(parent, className)
	for _, child in ipairs(parent:GetChildren()) do
		if child:IsA(className) then
			return child
		end
	end
	return nil
end

local function CalculateAbsoluteRotation(guiObject)
	local totalRotation = guiObject.Rotation
	local current = guiObject.Parent

	while current and current:IsA("GuiObject") do
		totalRotation = totalRotation + current.Rotation
		current = current.Parent
	end

	return totalRotation
end

local function SerializeGuiObject(guiObject, parentId, elementMap)
	local id = GenerateGUID()

	local data = {
		Id = id,
		ParentId = parentId or "",
		Name = guiObject.Name,
		ClassName = guiObject.ClassName,
		AbsoluteSize = SerializeVector2(guiObject.AbsoluteSize),
		AbsolutePosition = SerializeVector2(guiObject.AbsolutePosition),
		AbsoluteRotation = CalculateAbsoluteRotation(guiObject),
		Visible = guiObject.Visible,
		ZIndex = guiObject.ZIndex,
		Size = SerializeUDim2(guiObject.Size),
		Position = SerializeUDim2(guiObject.Position),
		AnchorPoint = SerializeVector2(guiObject.AnchorPoint),
		Rotation = guiObject.Rotation,
		BackgroundColor3 = SerializeColor3(guiObject.BackgroundColor3),
		BackgroundTransparency = guiObject.BackgroundTransparency,
		BorderSizePixel = guiObject.BorderSizePixel,
		BorderColor3 = SerializeColor3(guiObject.BorderColor3),
	}

	-- LayoutOrder
	if guiObject:FindFirstChild("LayoutOrder") then
		data.LayoutOrder = guiObject.LayoutOrder
	end

	-- UIGradient
	local uiGradient = FindChildOfClass(guiObject, "UIGradient")
	if uiGradient then
		data.UIGradient = SerializeUIGradient(uiGradient)
	end

	-- UIStroke
	local uiStroke = FindChildOfClass(guiObject, "UIStroke")
	if uiStroke then
		data.UIStroke = {
			Enabled = uiStroke.Enabled,
			Color = SerializeColor3(uiStroke.Color),
			Thickness = uiStroke.Thickness,
			Transparency = uiStroke.Transparency,
			ApplyStrokeMode = uiStroke.ApplyStrokeMode.Name,
		}

		local strokeGradient = FindChildOfClass(uiStroke, "UIGradient")
		if strokeGradient then
			data.UIStroke.UIGradient = SerializeUIGradient(strokeGradient)
		end
	end

	-- UICorner
	local uiCorner = FindChildOfClass(guiObject, "UICorner")
	if uiCorner then
		data.UICorner = {
			CornerRadius = SerializeUDim(uiCorner.CornerRadius),
		}
	end

	-- CanvasGroup specific
	if guiObject:IsA("CanvasGroup") then
		data.GroupTransparency = guiObject.GroupTransparency
		data.GroupColor3 = SerializeColor3(guiObject.GroupColor3)
	end

	-- ImageLabel/ImageButton specific
	if guiObject:IsA("ImageLabel") or guiObject:IsA("ImageButton") then
		data.Image = guiObject.Image
		data.ImageColor3 = SerializeColor3(guiObject.ImageColor3)
		data.ImageTransparency = guiObject.ImageTransparency
		data.ImageRectOffset = SerializeVector2(guiObject.ImageRectOffset)
		data.ImageRectSize = SerializeVector2(guiObject.ImageRectSize)
		data.ScaleType = guiObject.ScaleType.Name
	end

	-- ScrollingFrame specific
	if guiObject:IsA("ScrollingFrame") then
		data.CanvasSize = SerializeUDim2(guiObject.CanvasSize)
		data.ScrollBarThickness = guiObject.ScrollBarThickness
	end

	elementMap[id] = data

	-- Recursively serialize children
	for _, child in ipairs(guiObject:GetChildren()) do
		if child:IsA("GuiObject") then
			SerializeGuiObject(child, id, elementMap)
		end
	end

	return id
end

function Serializer.SerializeGuiObjects(guiObjects)
	local elementMap = {}

	for _, guiObject in ipairs(guiObjects) do
		SerializeGuiObject(guiObject, nil, elementMap)
	end

	-- Convert map to array
	local elements = {}
	for _, element in pairs(elementMap) do
		table.insert(elements, element)
	end

	return elements
end

return Serializer
